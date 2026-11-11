package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"unicode/utf16"

	"github.com/JetBrains/qodana-cli/internal/sarif"
)

const qodanaFingerprintV1 = "equalIndicator/v1"
const qodanaFingerprintV2 = "equalIndicator/v2"
const additionalFingerprintData = "equalIndicator/2/additionalData"
const stepFQNProperty = "step_fqn"

// safeHasher mirrors SafeHasher from Qodana's CommonFingerprints.kt. Guava
// writes integers and UTF-16 code units in little-endian order.
type safeHasher struct {
	data []byte
}

func (h *safeHasher) putInt(value int32) {
	h.data = binary.LittleEndian.AppendUint32(h.data, uint32(value))
}

func (h *safeHasher) putString(value string) {
	for _, char := range utf16.Encode([]rune(value)) {
		h.data = binary.LittleEndian.AppendUint16(h.data, char)
	}
}

func addQodanaFingerprints(result *sarif.Result) {
	if result.PartialFingerprints == nil {
		result.PartialFingerprints = make(map[string]string)
	}
	if result.PartialFingerprints[qodanaFingerprintV1] == "" {
		result.PartialFingerprints[qodanaFingerprintV1] = baselineEqualityV1(*result)
	}
	if result.PartialFingerprints[qodanaFingerprintV2] == "" {
		result.PartialFingerprints[qodanaFingerprintV2] = baselineEqualityV2(*result)
	}
}

func baselineEqualityV1(result sarif.Result) string {
	hasher := &safeHasher{}
	for _, location := range result.Locations {
		hashLocation(hasher, location)
	}
	for _, graph := range result.Graphs {
		for _, node := range graph.Nodes {
			hashNode(hasher, node)
		}
		for _, edge := range graph.Edges {
			hashEdge(hasher, edge)
		}
		putMessageText(hasher, graph.Description)
	}
	for _, codeFlow := range result.CodeFlows {
		hashCodeFlow(hasher, codeFlow, true)
	}
	hasher.putString(result.RuleId)
	putMessageText(hasher, result.Message)

	hash := sha256.Sum256(hasher.data)
	return hex.EncodeToString(hash[:])
}

func baselineEqualityV2(result sarif.Result) string {
	hasher := &safeHasher{}
	hasher.putString(result.RuleId)
	for _, location := range result.Locations {
		hashLocation(hasher, location)
	}
	if len(result.Locations) == 0 {
		putMessageText(hasher, result.Message)
	}
	for _, graph := range result.Graphs {
		for _, node := range graph.Nodes {
			hashNode(hasher, node)
		}
		for _, edge := range graph.Edges {
			hashEdge(hasher, edge)
		}
	}
	for _, codeFlow := range result.CodeFlows {
		hashCodeFlow(hasher, codeFlow, false)
	}
	if result.Properties != nil {
		if value, ok := result.Properties.AdditionalProperties[additionalFingerprintData]; ok {
			if hashCode, ok := javaHashCode(value); ok {
				hasher.putInt(hashCode)
			}
		}
	}

	return fingerprint2011Hex(hasher.data)
}

func fingerprint2011Hex(data []byte) string {
	fingerprint := fingerprint2011(data)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, fingerprint)
	return hex.EncodeToString(bytes)
}

func putMessageText(hasher *safeHasher, message *sarif.Message) {
	if message != nil {
		hasher.putString(message.Text)
	}
}

func hashLocation(hasher *safeHasher, location sarif.Location) {
	if physical := location.PhysicalLocation; physical != nil {
		if artifact := physical.ArtifactLocation; artifact != nil {
			hasher.putString(artifact.Uri)
			hasher.putString(artifact.UriBaseId)
		}
		if physical.Region != nil {
			hashRegion(hasher, *physical.Region)
		}
		if physical.ContextRegion != nil {
			hashRegion(hasher, *physical.ContextRegion)
		}
	}
	for _, logical := range location.LogicalLocations {
		hasher.putString(logical.Name)
		hasher.putString(logical.FullyQualifiedName)
		hasher.putString(logical.Kind)
	}
	for _, relationship := range location.Relationships {
		for _, kind := range relationship.Kinds {
			hasher.putString(kind)
		}
		hasher.putInt(int32(relationship.Target))
	}
}

func hashRegion(hasher *safeHasher, region sarif.Region) {
	hasher.putString(region.SourceLanguage)
	putOptionalInt(hasher, region.StartLine, region.HasStartLine())
	putOptionalInt(hasher, region.StartColumn, region.HasStartColumn())
	putOptionalInt(hasher, region.CharLength, region.HasCharLength())
	putOptionalInt(hasher, region.CharOffset, region.HasCharOffset())
	if region.Snippet != nil {
		hasher.putString(region.Snippet.Text)
	}
}

func putOptionalInt(hasher *safeHasher, value int64, present bool) {
	if present {
		hasher.putInt(int32(value))
	}
}

func hashEdge(hasher *safeHasher, edge sarif.Edge) {
	hasher.putString(edge.Id)
	hasher.putString(edge.SourceNodeId)
	hasher.putString(edge.TargetNodeId)
}

func hashNode(hasher *safeHasher, node sarif.Node) {
	hasher.putString(node.Id)
	if node.Location != nil {
		hashLocation(hasher, *node.Location)
	}
	hashProperties(hasher, node.Properties, "")
}

func hashCodeFlow(hasher *safeHasher, codeFlow sarif.CodeFlow, includeDescription bool) {
	if len(codeFlow.ThreadFlows) == 0 {
		if includeDescription {
			putMessageText(hasher, codeFlow.Message)
		}
		return
	}

	useCodeFlowDescription := len(codeFlow.ThreadFlows) == 1
	for _, threadFlow := range codeFlow.ThreadFlows {
		hashThreadFlow(hasher, threadFlow)
		if includeDescription {
			if threadFlow.Message != nil {
				hasher.putString(threadFlow.Message.Text)
			} else if useCodeFlowDescription {
				putMessageText(hasher, codeFlow.Message)
			}
		}
	}
}

func hashThreadFlow(hasher *safeHasher, threadFlow sarif.ThreadFlow) {
	locations := append([]sarif.ThreadFlowLocation(nil), threadFlow.Locations...)
	allHaveExecutionOrder := len(locations) > 0
	allHaveIndex := len(locations) > 0
	for _, location := range locations {
		allHaveExecutionOrder = allHaveExecutionOrder && location.HasExecutionOrder()
		allHaveIndex = allHaveIndex && location.HasIndex()
	}
	if allHaveExecutionOrder {
		sort.SliceStable(
			locations, func(i, j int) bool {
				return locations[i].ExecutionOrder < locations[j].ExecutionOrder
			},
		)
	} else if allHaveIndex {
		sort.SliceStable(
			locations, func(i, j int) bool {
				return locations[i].Index < locations[j].Index
			},
		)
	}

	nodeIDs := make([]string, len(locations))
	for i, location := range locations {
		nodeIDs[i] = threadFlowNodeID(location, i)
	}
	for i, location := range locations {
		hasher.putString(nodeIDs[i])
		if location.Location != nil {
			hashLocation(hasher, *location.Location)
		}
		hashProperties(hasher, location.Properties, stepFQNProperty)
	}
	for i := 0; i+1 < len(nodeIDs); i++ {
		hasher.putString(strconv.Itoa(i + 1))
		hasher.putString(nodeIDs[i])
		hasher.putString(nodeIDs[i+1])
	}
}

func threadFlowNodeID(location sarif.ThreadFlowLocation, index int) string {
	if location.HasExecutionOrder() {
		return strconv.FormatInt(location.ExecutionOrder, 10)
	}
	if location.HasIndex() {
		return strconv.FormatInt(location.Index, 10)
	}
	return strconv.Itoa(index + 1)
}

func hashProperties(hasher *safeHasher, properties *sarif.PropertyBag, excludedKey string) {
	if properties == nil {
		return
	}
	keys := make([]string, 0, len(properties.AdditionalProperties))
	for key := range properties.AdditionalProperties {
		if key != excludedKey {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		hasher.putString(key)
		if hashCode, ok := javaHashCode(properties.AdditionalProperties[key]); ok {
			hasher.putInt(hashCode)
		}
	}
}

func javaHashCode(value any) (int32, bool) {
	switch value := value.(type) {
	case nil:
		return 0, false
	case string:
		return javaStringHashCode(value), true
	case bool:
		if value {
			return 1231, true
		}
		return 1237, true
	case float64:
		bits := math.Float64bits(value)
		return int32(bits ^ (bits >> 32)), true
	case float32:
		return int32(math.Float32bits(value)), true
	case int:
		return int32(value), true
	case int8:
		return int32(value), true
	case int16:
		return int32(value), true
	case int32:
		return value, true
	case int64:
		return int32(value ^ (value >> 32)), true
	case uint:
		return int32(value), true
	case uint8:
		return int32(value), true
	case uint16:
		return int32(value), true
	case uint32:
		return int32(value), true
	case uint64:
		return int32(value ^ (value >> 32)), true
	case json.Number:
		if number, err := value.Int64(); err == nil {
			return int32(number ^ (number >> 32)), true
		}
		if number, err := value.Float64(); err == nil {
			bits := math.Float64bits(number)
			return int32(bits ^ (bits >> 32)), true
		}
	case []string:
		hash := int32(1)
		for _, item := range value {
			hash = 31*hash + javaStringHashCode(item)
		}
		return hash, true
	case []any:
		hash := int32(1)
		for _, item := range value {
			itemHash, _ := javaHashCode(item)
			hash = 31*hash + itemHash
		}
		return hash, true
	case map[string]any:
		hash := int32(0)
		for key, item := range value {
			itemHash, _ := javaHashCode(item)
			hash += javaStringHashCode(key) ^ itemHash
		}
		return hash, true
	}
	return 0, false
}

func javaStringHashCode(value string) int32 {
	var hash int32
	for _, char := range utf16.Encode([]rune(value)) {
		hash = 31*hash + int32(char)
	}
	return hash
}

// fingerprint2011 is a direct port of Guava's Hashing.fingerprint2011().
func fingerprint2011(data []byte) uint64 {
	length := len(data)
	var result uint64
	if length <= 32 {
		result = fingerprintMurmurHash64WithSeed(data, fingerprintK0^fingerprintK1^fingerprintK2)
	} else if length <= 64 {
		result = fingerprintLength33To64(data)
	} else {
		result = fingerprintFull(data)
	}

	u := fingerprintK0
	if length >= 8 {
		u = binary.LittleEndian.Uint64(data)
	}
	v := fingerprintK0
	if length >= 9 {
		v = binary.LittleEndian.Uint64(data[length-8:])
	}
	result = fingerprintHash128To64(result+v, u)
	if result == 0 || result == 1 {
		return result + ^uint64(1)
	}
	return result
}

const fingerprintK0 uint64 = 0xa5b85c5e198ed849
const fingerprintK1 uint64 = 0x8d58ac26afe12e47
const fingerprintK2 uint64 = 0xc47b6e9e3a970ed3
const fingerprintK3 uint64 = 0xc6a4a7935bd1e995

func fingerprintShiftMix(value uint64) uint64 {
	return value ^ (value >> 47)
}

func fingerprintHash128To64(high, low uint64) uint64 {
	a := (low ^ high) * fingerprintK3
	a ^= a >> 47
	b := (high ^ a) * fingerprintK3
	b ^= b >> 47
	return b * fingerprintK3
}

func fingerprintMurmurHash64WithSeed(data []byte, seed uint64) uint64 {
	length := len(data)
	lengthAligned := length &^ 7
	hash := seed ^ (uint64(length) * fingerprintK3)
	for i := 0; i < lengthAligned; i += 8 {
		loaded := binary.LittleEndian.Uint64(data[i:])
		value := fingerprintShiftMix(loaded*fingerprintK3) * fingerprintK3
		hash ^= value
		hash *= fingerprintK3
	}
	if remainder := length & 7; remainder != 0 {
		var value uint64
		for i := 0; i < remainder; i++ {
			value |= uint64(data[lengthAligned+i]) << (8 * i)
		}
		hash ^= value
		hash *= fingerprintK3
	}
	hash = fingerprintShiftMix(hash) * fingerprintK3
	return fingerprintShiftMix(hash)
}

func fingerprintLength33To64(data []byte) uint64 {
	length := len(data)
	z := binary.LittleEndian.Uint64(data[24:])
	a := binary.LittleEndian.Uint64(data) + (uint64(length)+binary.LittleEndian.Uint64(data[length-16:]))*fingerprintK0
	b := bits.RotateLeft64(a+z, -52)
	c := bits.RotateLeft64(a, -37)
	a += binary.LittleEndian.Uint64(data[8:])
	c += bits.RotateLeft64(a, -7)
	a += binary.LittleEndian.Uint64(data[16:])
	vFirst := a + z
	vSecond := b + bits.RotateLeft64(a, -31) + c
	a = binary.LittleEndian.Uint64(data[16:]) + binary.LittleEndian.Uint64(data[length-32:])
	z = binary.LittleEndian.Uint64(data[length-8:])
	b = bits.RotateLeft64(a+z, -52)
	c = bits.RotateLeft64(a, -37)
	a += binary.LittleEndian.Uint64(data[length-24:])
	c += bits.RotateLeft64(a, -7)
	a += binary.LittleEndian.Uint64(data[length-16:])
	wFirst := a + z
	wSecond := b + bits.RotateLeft64(a, -31) + c
	r := fingerprintShiftMix((vFirst+wSecond)*fingerprintK2 + (wFirst+vSecond)*fingerprintK0)
	return fingerprintShiftMix(r*fingerprintK0+vSecond) * fingerprintK2
}

func fingerprintWeakHashLength32WithSeeds(data []byte, seedA, seedB uint64) (uint64, uint64) {
	part1 := binary.LittleEndian.Uint64(data)
	part2 := binary.LittleEndian.Uint64(data[8:])
	part3 := binary.LittleEndian.Uint64(data[16:])
	part4 := binary.LittleEndian.Uint64(data[24:])
	seedA += part1
	seedB = bits.RotateLeft64(seedB+seedA+part4, -51)
	c := seedA
	seedA += part2 + part3
	seedB += bits.RotateLeft64(seedA, -23)
	return seedA + part4, seedB + c
}

func fingerprintFull(data []byte) uint64 {
	length := len(data)
	x := binary.LittleEndian.Uint64(data)
	y := binary.LittleEndian.Uint64(data[length-16:]) ^ fingerprintK1
	z := binary.LittleEndian.Uint64(data[length-56:]) ^ fingerprintK0
	v0, v1 := fingerprintWeakHashLength32WithSeeds(data[length-64:], uint64(length), y)
	w0, w1 := fingerprintWeakHashLength32WithSeeds(data[length-32:], uint64(length)*fingerprintK1, fingerprintK0)
	z += fingerprintShiftMix(v1) * fingerprintK1
	x = bits.RotateLeft64(z+x, -39) * fingerprintK1
	y = bits.RotateLeft64(y, -33) * fingerprintK1

	remaining := (length - 1) &^ 63
	offset := 0
	for {
		x = bits.RotateLeft64(x+y+v0+binary.LittleEndian.Uint64(data[offset+16:]), -37) * fingerprintK1
		y = bits.RotateLeft64(y+v1+binary.LittleEndian.Uint64(data[offset+48:]), -42) * fingerprintK1
		x ^= w1
		y ^= v0
		z = bits.RotateLeft64(z^w0, -33)
		v0, v1 = fingerprintWeakHashLength32WithSeeds(data[offset:], v1*fingerprintK1, x+w0)
		w0, w1 = fingerprintWeakHashLength32WithSeeds(data[offset+32:], z+w1, y)
		z, x = x, z
		offset += 64
		remaining -= 64
		if remaining == 0 {
			break
		}
	}
	return fingerprintHash128To64(
		fingerprintHash128To64(v0, w0)+fingerprintShiftMix(y)*fingerprintK1+z,
		fingerprintHash128To64(v1, w1)+x,
	)
}
