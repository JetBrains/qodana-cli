// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
)

const maxFileSize = 8 << 20

// Stable artifact IDs are lowercase so distinct capability scopes cannot alias
// on case-insensitive filesystems. Source filenames inside examples may use case.
var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Step struct {
	Skill string `json:"skill"`
	Title string `json:"title"`
}

type Task struct {
	ID         string   `json:"id"`
	ParentID   string   `json:"parentId,omitempty"`
	Skill      string   `json:"skill"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	AgentID    string   `json:"agentId,omitempty"`
	Result     string   `json:"result,omitempty"`
	Operations []string `json:"operations,omitempty"`
	Scope      []string `json:"scope,omitempty"`
}

type Plan struct {
	ID       string `json:"id"`
	Request  string `json:"request"`
	Revision int    `json:"revision"`
	Tasks    []Task `json:"tasks"`
}

// PlanCreation returns the manager capability once, separately from persisted state.
type PlanCreation struct {
	Plan  *Plan  `json:"plan"`
	Token string `json:"token"`
}

type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Hash    string `json:"hash"`
}

type Delegation struct {
	Token  string `json:"token"`
	TaskID string `json:"taskId"`
}

type capability struct {
	skill, taskID, parent string
	operations, scope     []string
}

// Store serializes state mutations, pins all I/O to an os.Root and holds an OS
// lock for its lifetime. Bearer secrets live only in memory as SHA-256 hashes.
// The host must deny agents direct filesystem writes to this root.
type Store struct {
	mu             sync.Mutex
	root           *os.Root
	lock           *os.File
	policies       map[string]Policy
	grants         map[string]capability
	plan           *Plan
	managerClaimed bool
	closed         bool
}

func NewStore(directory string) (*Store, error) {
	policies, err := register(DefaultRegistry())
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	s := &Store{root: root, policies: policies, grants: make(map[string]capability)}
	fail := func(err error) (*Store, error) { _ = s.Close(); return nil, err }
	if err := s.safePath(".edict-mcp.lock"); err != nil {
		return fail(err)
	}
	s.lock, err = root.OpenFile(".edict-mcp.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fail(err)
	}
	if err := lockFile(s.lock); err != nil {
		return fail(fmt.Errorf("state is already owned by another edict-mcp server: %w", err))
	}
	if err := s.restorePlan(); err != nil {
		return fail(err)
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.grants = nil
	var err error
	if s.lock != nil {
		err = s.lock.Close()
	}
	return errors.Join(err, s.root.Close())
}

func (s *Store) Registry() []Policy { return DefaultRegistry() }

func (s *Store) Plan() *Plan {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePlan(s.plan)
}

func clonePlan(p *Plan) *Plan {
	if p == nil {
		return nil
	}
	data, _ := json.Marshal(p)
	var copy Plan
	_ = json.Unmarshal(data, &copy)
	return &copy
}

// CreatePlan grants the first successful caller manager authority. The claim is
// serialized with plan persistence and cannot be repeated during this store's
// lifetime, even after all tasks finish. After a restart, a matching unfinished
// plan is resumed with a fresh token instead of losing its progress.
func (s *Store) CreatePlan(request string, steps []Step) (*PlanCreation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("edict-mcp store is closed")
	}
	if s.managerClaimed {
		return nil, errors.New("edict_plan_create has already succeeded for this server; use the existing manager token and plan")
	}
	if strings.TrimSpace(request) == "" || len(steps) == 0 || len(steps) > 1000 {
		return nil, errors.New("a request and 1..1000 steps are required")
	}
	c := capability{skill: "edict_manager", operations: slices.Clone(s.policies["edict_manager"].Operations), scope: []string{"inbox", "clusters", "inspections"}}
	p := &Plan{ID: randomID(), Request: request}
	for _, step := range steps {
		if err := s.allowedChild(c, step.Skill, step.Title); err != nil {
			return nil, err
		}
		p.Tasks = append(p.Tasks, Task{ID: randomID(), Skill: step.Skill, Title: step.Title, Status: "pending"})
	}
	resume := s.plan != nil && slices.ContainsFunc(s.plan.Tasks, func(task Task) bool {
		return task.Status != "completed" && task.Status != "failed"
	})
	if resume {
		var existingSteps []Step
		for _, task := range s.plan.Tasks {
			if task.ParentID == "" {
				existingSteps = append(existingSteps, Step{Skill: task.Skill, Title: task.Title})
			}
		}
		if request != s.plan.Request || !slices.Equal(steps, existingSteps) {
			return nil, errors.New("resume the existing unfinished plan with its original request and top-level steps")
		}
	} else {
		if err := s.savePlan(p); err != nil {
			return nil, err
		}
	}
	token := randomID()
	s.grants[hash(token)] = c
	s.managerClaimed = true
	return &PlanCreation{Plan: clonePlan(s.plan), Token: token}, nil
}

func (s *Store) AddTask(token, skill, title string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.authorize(token)
	if err != nil {
		return Task{}, err
	}
	if s.plan == nil {
		return Task{}, errors.New("create a plan first")
	}
	if len(s.plan.Tasks) >= 10000 {
		return Task{}, errors.New("plan task limit reached")
	}
	if err := s.allowedChild(c, skill, title); err != nil {
		return Task{}, err
	}
	p := clonePlan(s.plan)
	task := Task{ID: randomID(), ParentID: c.taskID, Skill: skill, Title: title, Status: "pending"}
	p.Tasks = append(p.Tasks, task)
	if err := s.savePlan(p); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *Store) allowedChild(c capability, skill, title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("task title is required")
	}
	if !slices.Contains(s.policies[c.skill].Delegates, skill) {
		return fmt.Errorf("%s cannot call managed skill %s", c.skill, skill)
	}
	return nil
}

func (s *Store) Delegate(token, taskID string, operations, scope []string) (Delegation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.authorize(token)
	if err != nil {
		return Delegation{}, err
	}
	p := clonePlan(s.plan)
	task := findTask(p, taskID)
	if task == nil || task.ParentID != c.taskID {
		return Delegation{}, errors.New("task is not a direct child of this capability")
	}
	if err := s.allowedChild(c, task.Skill, task.Title); err != nil {
		return Delegation{}, err
	}
	if task.Status != "pending" && task.Status != "failed" {
		return Delegation{}, errors.New("task is already delegated or completed")
	}
	for _, op := range operations {
		if !slices.Contains(c.operations, op) || !slices.Contains(s.policies[task.Skill].Operations, op) {
			return Delegation{}, fmt.Errorf("cannot delegate operation %s", op)
		}
	}
	for _, prefix := range scope {
		if err := validPath(prefix); err != nil {
			return Delegation{}, err
		}
		if !covered(c.scope, prefix) {
			return Delegation{}, fmt.Errorf("cannot widen scope to %s", prefix)
		}
	}
	if len(operations) > 0 && len(scope) == 0 {
		return Delegation{}, errors.New("mutation operations require explicit scope")
	}
	task.Status, task.AgentID, task.Result = "delegated", "", ""
	task.Operations, task.Scope = slices.Clone(operations), slices.Clone(scope)
	secret := randomID()
	if err := s.savePlan(p); err != nil {
		return Delegation{}, err
	}
	s.grants[hash(secret)] = capability{skill: task.Skill, taskID: task.ID, parent: hash(token), operations: slices.Clone(operations), scope: slices.Clone(scope)}
	return Delegation{Token: secret, TaskID: task.ID}, nil
}

func (s *Store) StartTask(token, agentID string) (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.lookup(token)
	if err != nil {
		return nil, err
	}
	p := clonePlan(s.plan)
	task := findTask(p, c.taskID)
	if task == nil || task.Status != "delegated" {
		return nil, errors.New("only a delegated worker can start its task")
	}
	if strings.TrimSpace(agentID) == "" {
		return nil, errors.New("subagent ID is required")
	}
	for _, existing := range p.Tasks {
		if existing.AgentID == agentID {
			return nil, errors.New("each task requires a fresh subagent ID")
		}
	}
	task.Status, task.AgentID = "running", agentID
	if err := s.savePlan(p); err != nil {
		return nil, err
	}
	return clonePlan(s.plan), nil
}

func (s *Store) FinishTask(token, status, result string) (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.authorize(token)
	if err != nil {
		return nil, err
	}
	if c.taskID == "" {
		return nil, errors.New("manager must finish tasks through their workers")
	}
	if status != "completed" && status != "failed" {
		return nil, errors.New("status must be completed or failed")
	}
	if strings.TrimSpace(result) == "" {
		return nil, errors.New("task result is required")
	}
	p := clonePlan(s.plan)
	for i := range p.Tasks {
		task := &p.Tasks[i]
		if !descendant(p, task.ID, c.taskID) {
			continue
		}
		if status == "completed" && task.Status != "completed" {
			return nil, errors.New("complete all subtasks before completing their parent")
		}
		if status == "failed" && task.Status != "completed" {
			task.Status, task.Result = "failed", "Parent task failed: "+result
		}
	}
	task := findTask(p, c.taskID)
	task.Status, task.Result = status, result
	if err := s.savePlan(p); err != nil {
		return nil, err
	}
	for key, grant := range s.grants {
		if grant.taskID == c.taskID || descendant(s.plan, grant.taskID, c.taskID) {
			delete(s.grants, key)
		}
	}
	return clonePlan(s.plan), nil
}

// CancelTask lets a coordinator account for a worker that exited without a
// receipt. It cannot report success on the worker's behalf.
func (s *Store) CancelTask(token, taskID, result string) (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.authorize(token)
	if err != nil {
		return nil, err
	}
	p := clonePlan(s.plan)
	task := findTask(p, taskID)
	if task == nil || task.ParentID != c.taskID {
		return nil, errors.New("only the direct coordinator can cancel a task")
	}
	if task.Status == "completed" || task.Status == "failed" || strings.TrimSpace(result) == "" {
		return nil, errors.New("cancellation requires an unfinished task and a reason")
	}
	task.Status, task.Result = "failed", result
	for i := range p.Tasks {
		child := &p.Tasks[i]
		if descendant(p, child.ID, taskID) && child.Status != "completed" {
			child.Status, child.Result = "failed", "Parent task cancelled: "+result
		}
	}
	if err := s.savePlan(p); err != nil {
		return nil, err
	}
	for key, grant := range s.grants {
		if grant.taskID == taskID || descendant(p, grant.taskID, taskID) {
			delete(s.grants, key)
		}
	}
	return clonePlan(s.plan), nil
}

func descendant(p *Plan, taskID, ancestor string) bool {
	for task := findTask(p, taskID); task != nil && task.ParentID != ""; task = findTask(p, task.ParentID) {
		if task.ParentID == ancestor {
			return true
		}
	}
	return false
}

func findTask(p *Plan, id string) *Task {
	if p != nil {
		for i := range p.Tasks {
			if p.Tasks[i].ID == id {
				return &p.Tasks[i]
			}
		}
	}
	return nil
}

func (s *Store) lookup(token string) (capability, error) {
	if s.closed {
		return capability{}, errors.New("edict-mcp store is closed")
	}
	c, ok := s.grants[hash(token)]
	if !ok {
		return capability{}, errors.New("invalid or revoked capability")
	}
	if c.parent != "" {
		if _, ok := s.grants[c.parent]; !ok {
			return capability{}, errors.New("parent capability is revoked")
		}
	}
	return c, nil
}

func (s *Store) authorize(token string) (capability, error) {
	c, err := s.lookup(token)
	if err != nil {
		return c, err
	}
	if c.taskID != "" {
		task := findTask(s.plan, c.taskID)
		if task == nil || task.Status != "running" {
			return capability{}, errors.New("worker must start its delegated task first")
		}
	}
	return c, nil
}

func (s *Store) Read(name string) (File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !readable(name) {
		return File{}, errors.New("not a persisted Edict artifact")
	}
	return s.read(name)
}

func (s *Store) read(name string) (File, error) {
	if err := s.safePath(name); err != nil {
		return File{}, err
	}
	f, err := s.root.Open(name)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return File{}, err
	}
	if len(data) > maxFileSize {
		return File{}, errors.New("artifact exceeds 8 MiB")
	}
	return File{Path: name, Content: string(data), Hash: hash(string(data))}, nil
}

func (s *Store) List(prefix string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prefix != "" {
		if err := validPath(prefix); err != nil {
			return nil, err
		}
	}
	var paths []string
	for _, directory := range []string{"inbox", "clusters", "inspections", "plans"} {
		if prefix != "" && !covered([]string{directory}, prefix) && !covered([]string{prefix}, directory) {
			continue
		}
		if err := s.safePath(directory); err != nil {
			return nil, err
		}
		err := fs.WalkDir(s.root.FS(), directory, func(name string, d fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) && name == directory {
				return nil
			}
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink is forbidden: %s", name)
			}
			if !d.IsDir() && readable(name) && (prefix == "" || covered([]string{prefix}, name)) {
				paths = append(paths, name)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	slices.Sort(paths)
	return paths, nil
}

func (s *Store) Write(token, name, content, expectedHash string) (File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.authorizeWrite(token, name, false)
	if err != nil {
		return File{}, err
	}
	if len(content) > maxFileSize {
		return File{}, errors.New("artifact exceeds 8 MiB")
	}
	if strings.HasSuffix(name, ".json") && !json.Valid([]byte(content)) {
		return File{}, errors.New("artifact must contain valid JSON")
	}
	old, err := s.compare(name, expectedHash)
	if err != nil {
		return File{}, err
	}
	if strings.HasSuffix(name, "/description.json") {
		var description struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(content), &description) != nil || description.ID != strings.Split(name, "/")[1] {
			return File{}, errors.New("cluster description ID must match its directory")
		}
	}
	if c.skill == "edict-next-code-example" && operation(name) == "cluster.signal.write" {
		if err := exampleLinkOnly(old.Content, content); err != nil {
			return File{}, err
		}
	}
	if op := operation(name); op == "inbox.write" || op == "cluster.signal.write" {
		if err := validateSignal(name, content); err != nil {
			return File{}, err
		}
	}
	if err := s.atomicWrite(name, []byte(content)); err != nil {
		return File{}, err
	}
	return File{Path: name, Content: content, Hash: hash(content)}, nil
}

func (s *Store) Delete(token, name, expectedHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.authorizeWrite(token, name, true)
	if err != nil {
		return err
	}
	if c.skill == "edict-next-code-example" && operation(name) == "cluster.signal.write" {
		return errors.New("example workers cannot delete signals")
	}
	if expectedHash == "" {
		return errors.New("deletion requires the existing artifact hash")
	}
	if _, err := s.compare(name, expectedHash); err != nil {
		return err
	}
	return s.root.Remove(name)
}

func (s *Store) authorizeWrite(token, name string, deleting bool) (capability, error) {
	c, err := s.authorize(token)
	if err != nil {
		return c, err
	}
	op := operation(name)
	if deleting && op == "inbox.write" {
		op = "inbox.delete"
	}
	if op == "" || !slices.Contains(c.operations, op) || !slices.Contains(s.policies[c.skill].Writes, op) || !covered(c.scope, name) {
		return capability{}, fmt.Errorf("%s cannot modify %s", c.skill, name)
	}
	if err := s.safePath(name); err != nil {
		return capability{}, err
	}
	return c, nil
}

func (s *Store) compare(name, expectedHash string) (File, error) {
	old, err := s.read(name)
	if errors.Is(err, os.ErrNotExist) {
		if expectedHash == "" {
			return File{}, nil
		}
		return File{}, errors.New("artifact no longer exists; read it again")
	}
	if err != nil {
		return File{}, err
	}
	if old.Hash != expectedHash {
		return File{}, errors.New("artifact hash conflict; read it again before modifying")
	}
	return old, nil
}

func exampleLinkOnly(before, after string) error {
	var old, next map[string]json.RawMessage
	if json.Unmarshal([]byte(before), &old) != nil || json.Unmarshal([]byte(after), &next) != nil || old == nil || next == nil {
		return errors.New("example assignment requires an existing signal object")
	}
	var id string
	if json.Unmarshal(next["syntheticExampleId"], &id) != nil || !identifier.MatchString(id) {
		return errors.New("syntheticExampleId must name an example")
	}
	delete(old, "syntheticExampleId")
	delete(next, "syntheticExampleId")
	a, _ := json.Marshal(old)
	b, _ := json.Marshal(next)
	// Normalize whitespace within nested JSON values, preserving numeric values.
	var ca, cb strings.Builder
	compact := func(data []byte, out *strings.Builder) {
		var value any
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.UseNumber()
		_ = dec.Decode(&value)
		normalized, _ := json.Marshal(value)
		out.Write(normalized)
	}
	compact(a, &ca)
	compact(b, &cb)
	if ca.String() != cb.String() {
		return errors.New("example workers may change only syntheticExampleId in a signal")
	}
	return nil
}

func operation(name string) string {
	if validPath(name) != nil {
		return ""
	}
	p := strings.Split(name, "/")
	if len(p) == 2 && p[0] == "inbox" && strings.HasSuffix(p[1], ".json") && identifier.MatchString(strings.TrimSuffix(p[1], ".json")) {
		return "inbox.write"
	}
	if len(p) == 2 && p[0] == "inspections" {
		for _, suffix := range []string{".candidate.kts", ".inspection.kts"} {
			if strings.HasSuffix(p[1], suffix) && identifier.MatchString(strings.TrimSuffix(p[1], suffix)) {
				return "inspection.write"
			}
		}
	}
	if len(p) >= 3 && p[0] == "clusters" && identifier.MatchString(p[1]) {
		if len(p) == 3 && (p[2] == "description.json" || p[2] == "history.md") {
			return "cluster.write"
		}
		if len(p) == 4 && p[2] == "signals" && strings.HasSuffix(p[3], ".json") && identifier.MatchString(strings.TrimSuffix(p[3], ".json")) {
			return "cluster.signal.write"
		}
		if len(p) >= 5 && p[2] == "synthetic-examples" && identifier.MatchString(p[3]) {
			if len(p) == 5 && p[4] == "metadata.json" || len(p) >= 6 && p[4] == "project" {
				return "example.write"
			}
		}
	}
	return ""
}

func readable(name string) bool {
	return operation(name) != "" || validPath(name) == nil && strings.HasPrefix(name, "plans/") && strings.Count(name, "/") == 1 && strings.HasSuffix(name, ".json")
}

func validPath(name string) error {
	if name == "" || !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00\r\n") {
		return fmt.Errorf("invalid relative artifact path %q", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return fmt.Errorf("ambiguous artifact path %q", name)
		}
	}
	return nil
}

func covered(scope []string, name string) bool {
	for _, prefix := range scope {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return true
		}
	}
	return false
}

func (s *Store) safePath(name string) error {
	if s.closed {
		return errors.New("edict-mcp store is closed")
	}
	if err := validPath(name); err != nil {
		return err
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := s.root.Lstat(strings.Join(parts[:i+1], "/"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are forbidden: %s", name)
		}
		if i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular artifact path: %s", name)
		}
	}
	return nil
}

func (s *Store) atomicWrite(name string, data []byte) error {
	if err := s.safePath(name); err != nil {
		return err
	}
	if err := s.root.MkdirAll(path.Dir(name), 0o700); err != nil {
		return err
	}
	temporary := path.Join(path.Dir(name), ".edict-tmp-"+randomID())
	f, err := s.root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer s.root.Remove(temporary)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	if err := errors.Join(writeErr, syncErr, f.Close()); err != nil {
		return err
	}
	return s.root.Rename(temporary, name)
}

func (s *Store) savePlan(p *Plan) error {
	p.Revision++
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > maxFileSize {
		return errors.New("execution plan exceeds 8 MiB; use shorter task results")
	}
	if err := s.atomicWrite("plans/"+p.ID+".json", append(data, '\n')); err != nil {
		return err
	}
	if s.plan == nil || s.plan.ID != p.ID {
		if err := s.atomicWrite(".edict-mcp-current", []byte(p.ID)); err != nil {
			return err
		}
	}
	s.plan = p
	return nil
}

func (s *Store) restorePlan() error {
	if err := s.safePath(".edict-mcp-current"); err != nil {
		return err
	}
	id, err := s.root.ReadFile(".edict-mcp-current")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !identifier.Match(id) {
		return errors.New("invalid active plan ID")
	}
	file, err := s.read("plans/" + string(id) + ".json")
	if err != nil {
		return err
	}
	var p Plan
	if err := json.Unmarshal([]byte(file.Content), &p); err != nil {
		return fmt.Errorf("invalid saved plan: %w", err)
	}
	if p.ID != string(id) || p.Revision < 1 {
		return errors.New("saved plan identity is invalid")
	}
	seen := make(map[string]bool)
	changed := false
	for i := range p.Tasks {
		task := &p.Tasks[i]
		if !identifier.MatchString(task.ID) || seen[task.ID] || task.ParentID != "" && !seen[task.ParentID] || s.policies[task.Skill].Name == "" {
			return errors.New("invalid saved task graph")
		}
		seen[task.ID] = true
		switch task.Status {
		case "running", "delegated":
			task.Status, task.AgentID, task.Result = "pending", "", "Interrupted by server restart; delegate to a fresh worker to resume."
			changed = true
		case "pending", "completed", "failed":
		default:
			return errors.New("invalid saved task status")
		}
	}
	s.plan = &p
	if changed {
		return s.savePlan(&p)
	}
	return nil
}

func randomID() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
