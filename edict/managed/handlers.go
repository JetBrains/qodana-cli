// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package managed

type toolHandlers struct {
	store *Store
	log   *activityLogger
}

func (h toolHandlers) registry(in callerInput) (any, error) {
	caller := h.taskForToken(in.Token)
	skills := h.store.Registry()
	h.log.printf(caller, "Read skill registry: %d skills available", len(skills))
	return map[string]any{"skills": skills}, nil
}

func (h toolHandlers) read(in readInput) (any, error) {
	caller := h.taskForToken(in.Token)
	file, err := h.store.Read(in.Path)
	if err != nil {
		h.log.printf(caller, "Read %q failed: %s", h.log.text(in.Path), h.log.text(err.Error()))
	} else {
		h.log.printf(caller, "Read %q (%d bytes)", h.log.text(file.Path), len(file.Content))
	}
	return file, err
}

func (h toolHandlers) list(in listInput) (any, error) {
	caller := h.taskForToken(in.Token)
	paths, err := h.store.List(in.Prefix)
	if err != nil {
		h.log.printf(caller, "List files under %q failed: %s", h.log.text(in.Prefix), h.log.text(err.Error()))
	} else {
		prefix := in.Prefix
		if prefix == "" {
			prefix = "."
		}
		h.log.printf(caller, "Listed files under %q: %d found", h.log.text(prefix), len(paths))
	}
	return map[string]any{"paths": paths}, err
}

func (h toolHandlers) planGet(in callerInput) (any, error) {
	caller := h.taskForToken(in.Token)
	plan := h.store.Plan()
	if plan == nil {
		h.log.printf(caller, "Read plan: no plan created yet")
	} else {
		counts := make(map[string]int)
		for _, task := range plan.Tasks {
			counts[task.Status]++
		}
		h.log.printf(caller, "Read plan: %d pending, %d delegated, %d running, %d completed, %d failed",
			counts["pending"], counts["delegated"], counts["running"], counts["completed"], counts["failed"])
	}
	return map[string]any{"plan": plan}, nil
}

func (h toolHandlers) planCreate(in createPlanInput) (any, error) {
	created, err := h.store.CreatePlan(in.Request, in.Steps)
	if err != nil {
		h.log.printf(Task{Skill: "anonymous"}, "Create plan %q failed: %s", h.log.text(in.Request), h.log.text(err.Error()))
	} else {
		h.log.remember(created.Token, Task{Skill: "edict_manager"})
		h.log.printf(Task{Skill: "edict_manager"}, "Plan ready: %q; manager assigned", h.log.text(created.Plan.Request))
	}
	return created, err
}

func (h toolHandlers) taskAdd(in addTaskInput) (any, error) {
	caller := h.taskForToken(in.Token)
	task, err := h.store.AddTask(in.Token, in.Skill, in.Title)
	if err != nil {
		h.log.printf(caller, "Add task %q (%s) failed: %s", h.log.text(in.Title), displaySkill(h.log.text(in.Skill)), h.log.text(err.Error()))
	} else {
		h.log.printf(caller, "Added task %s", h.log.target(task))
	}
	return task, err
}

func (h toolHandlers) delegate(in delegateInput) (any, error) {
	caller := h.taskForToken(in.Token)
	task := h.taskForID(in.TaskID)
	grant, err := h.store.Delegate(in.Token, in.TaskID, in.Operations, in.Scope)
	if err != nil {
		h.log.printf(caller, "Delegate task %s failed: %s", h.log.target(task), h.log.text(err.Error()))
	} else {
		h.log.remember(grant.Token, task)
		h.log.printf(caller, "Delegated task %s", h.log.target(task))
	}
	return grant, err
}

func (h toolHandlers) taskStart(in startTaskInput) (any, error) {
	task := h.taskForToken(in.Token)
	plan, err := h.store.StartTask(in.Token, in.AgentID, in.Skill)
	if err != nil {
		h.log.printf(task, "Start task %q failed: %s", h.log.text(task.Title), h.log.text(err.Error()))
	} else {
		h.log.printf(task, "Started task %q", h.log.text(task.Title))
	}
	return plan, err
}

func (h toolHandlers) taskFinish(in finishTaskInput) (any, error) {
	// Capture the title before finishing revokes the worker's capability.
	task := h.taskForToken(in.Token)
	plan, err := h.store.FinishTask(in.Token, in.Status, in.Result)
	if err != nil {
		h.log.printf(task, "Finish task %q failed: %s", h.log.text(task.Title), h.log.text(err.Error()))
	} else {
		h.log.printf(task, "Task %q %s: %s", h.log.text(task.Title), in.Status, h.log.result(in.Result))
	}
	return plan, err
}

func (h toolHandlers) taskCancel(in cancelTaskInput) (any, error) {
	caller := h.taskForToken(in.Token)
	task := h.taskForID(in.TaskID)
	plan, err := h.store.CancelTask(in.Token, in.TaskID, in.Result)
	if err != nil {
		h.log.printf(caller, "Cancel task %s failed: %s", h.log.target(task), h.log.text(err.Error()))
	} else {
		h.log.printf(caller, "Cancelled task %s: %s", h.log.target(task), h.log.result(in.Result))
	}
	return plan, err
}

func (h toolHandlers) stateWrite(in writeInput) (any, error) {
	task := h.taskForToken(in.Token)
	file, err := h.store.Write(in.Token, in.Path, in.Content, in.ExpectedHash)
	if err != nil {
		h.log.printf(task, "Write %q failed: %s", h.log.text(in.Path), h.log.text(err.Error()))
	} else {
		h.log.printf(task, "Wrote %q (%d bytes)", h.log.text(file.Path), len(file.Content))
	}
	return file, err
}

func (h toolHandlers) stateDelete(in deleteInput) (any, error) {
	task := h.taskForToken(in.Token)
	err := h.store.Delete(in.Token, in.Path, in.ExpectedHash)
	if err != nil {
		h.log.printf(task, "Delete %q failed: %s", h.log.text(in.Path), h.log.text(err.Error()))
	} else {
		h.log.printf(task, "Deleted %q", h.log.text(in.Path))
	}
	return map[string]any{"deleted": err == nil}, err
}

// Snapshot display metadata before a mutation can revoke the capability.
func (h toolHandlers) taskForToken(token string) Task {
	if token == "" {
		return Task{Skill: "anonymous"}
	}
	if task, ok := h.log.task(token); ok {
		return task
	}
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	grant, err := h.store.lookup(token)
	if err != nil {
		return Task{Skill: "unknown"}
	}
	task := Task{ID: grant.taskID, Skill: grant.skill}
	if stored := findTask(h.store.plan, grant.taskID); stored != nil {
		task.Title = stored.Title
	}
	h.log.remember(token, task)
	return task
}

func (h toolHandlers) taskForID(id string) Task {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	if task := findTask(h.store.plan, id); task != nil {
		return Task{ID: task.ID, Skill: task.Skill, Title: task.Title}
	}
	return Task{Skill: "unknown"}
}
