package shlex

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// complexityExponent approximates the exponent k such that the input
// datapoints are best described by the curve y = c·x^k.
//
// Background: an algorithm running in O(n^k) time (or allocating
// O(n^k) bytes) produces measurements that follow a power-law curve.
// Taking the log of both axes turns the curve into a straight line
// whose slope is k. We fit that line by ordinary least squares.
//
// Interpretation:
//   - k ≈ 1          → linear       (O(n))
//   - k just above 1 → linearithmic (O(n log n))
//   - k ≈ 2          → quadratic    (O(n²))
//
// Preconditions: xs and ys have equal length ≥ 2, every entry is
// strictly positive, and xs contains at least two distinct values.
func complexityExponent(xs, ys []float64) float64 {
	if len(xs) != len(ys) {
		panic("complexityExponent: xs and ys must have equal length")
	}
	if len(xs) < 2 {
		panic("complexityExponent: at least 2 datapoints required")
	}

	logX := make([]float64, len(xs))
	logY := make([]float64, len(ys))
	for i := range xs {
		logX[i] = math.Log(xs[i])
		logY[i] = math.Log(ys[i])
	}
	meanLogX := mean(logX)
	meanLogY := mean(logY)

	// Least-squares slope of the line that fits (logX, logY):
	//   slope = sum((logX_i - meanLogX) * (logY_i - meanLogY))
	//         / sum((logX_i - meanLogX)^2)
	var slopeNumerator, slopeDenominator float64
	for i := range logX {
		logXDeviation := logX[i] - meanLogX
		logYDeviation := logY[i] - meanLogY
		slopeNumerator += logXDeviation * logYDeviation
		slopeDenominator += logXDeviation * logXDeviation
	}
	if slopeDenominator == 0 {
		panic("complexityExponent: all xs are equal; slope is undefined")
	}
	return slopeNumerator / slopeDenominator
}

func mean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func TestComplexityExponent(t *testing.T) {
	cases := []struct {
		name         string
		curve        func(x float64) float64
		wantExponent float64
	}{
		{"linear", func(x float64) float64 { return 2 * x }, 1.0},
		{"quadratic", func(x float64) float64 { return 3 * x * x }, 2.0},
		{"sqrt", func(x float64) float64 { return 5 * math.Sqrt(x) }, 0.5},
		{"cubic", func(x float64) float64 { return x * x * x }, 3.0},
	}
	xs := []float64{10, 30, 100, 300, 1000}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ys := make([]float64, len(xs))
			for i, x := range xs {
				ys[i] = tt.curve(x)
			}
			assert.InDelta(t, tt.wantExponent, complexityExponent(xs, ys), 0.01,
				"exponent mismatch for %s", tt.name)
		})
	}
}

// TestSplit_IsNotQuadratic measures Split across a range of input
// sizes and fits a power-law curve to each metric. A correct (linear)
// implementation has exponent ≈ 1; a quadratic regression pushes it
// toward 2. The test fails if either exponent is ≥ 1.5 — strictly
// closer to quadratic than to linear.
func TestSplit_IsNotQuadratic(t *testing.T) {
	if testing.Short() {
		t.Skip("complexity test — skipped under -short")
	}

	const (
		repeatUnit        = `foo "bar baz" 'qux' `
		samplesPerSize    = 7
		exponentThreshold = 1.5
	)
	inputSizes := []int{10_000, 30_000, 100_000, 300_000, 1_000_000}

	// Warmup: run a couple of calls up front so lazy allocator init
	// and first-fault costs don't bias the smallest datapoint.
	for range 2 {
		_, _ = Split(strings.Repeat(repeatUnit, inputSizes[0]))
	}

	// Pre-build inputs once. Repeated strings.Repeat calls would
	// themselves dwarf the Split allocations we want to measure.
	inputs := make([]string, len(inputSizes))
	for i, size := range inputSizes {
		inputs[i] = strings.Repeat(repeatUnit, size)
	}

	runtimes := make([][]float64, len(inputSizes))
	allocations := make([][]float64, len(inputSizes))
	for i := range inputSizes {
		runtimes[i] = make([]float64, samplesPerSize)
		allocations[i] = make([]float64, samplesPerSize)
	}

	// Outer: samples. Inner: sizes. Interleaving decorrelates slow
	// drift (thermal, scheduler pressure) from input size.
	for sampleIdx := range samplesPerSize {
		for sizeIdx, input := range inputs {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			start := time.Now()
			tokens, err := Split(input)
			elapsed := time.Since(start)
			runtime.ReadMemStats(&after)

			require.NoError(t, err)
			// require.Len is load-bearing: it keeps tokens reachable
			// past ReadMemStats(&after) so escape analysis cannot
			// shrink Split's allocation footprint. Do not simplify to
			// `_, err := Split(input)`.
			require.Len(t, tokens, 3*inputSizes[sizeIdx])
			require.Positive(t, elapsed,
				"measured zero elapsed time — clock resolution too coarse for input size %d",
				inputSizes[sizeIdx])
			require.GreaterOrEqual(t, after.TotalAlloc, before.TotalAlloc,
				"TotalAlloc went backwards — runtime counter assumption violated")

			runtimes[sizeIdx][sampleIdx] = float64(elapsed.Nanoseconds())
			allocations[sizeIdx][sampleIdx] = float64(after.TotalAlloc - before.TotalAlloc)
		}
	}

	sizes := make([]float64, len(inputSizes))
	minNanos := make([]float64, len(inputSizes))
	medianBytes := make([]float64, len(inputSizes))
	for i, size := range inputSizes {
		sizes[i] = float64(size)
		minNanos[i] = slices.Min(runtimes[i])
		sorted := slices.Clone(allocations[i])
		slices.Sort(sorted)
		medianBytes[i] = sorted[samplesPerSize/2]
	}

	formatRows := func() string {
		rows := make([]string, len(inputSizes))
		for i := range inputSizes {
			rows[i] = fmt.Sprintf("  size=%d  min_ns=%.0f  median_bytes=%.0f",
				inputSizes[i], minNanos[i], medianBytes[i])
		}
		return strings.Join(rows, "\n")
	}

	timeExponent := complexityExponent(sizes, minNanos)
	spaceExponent := complexityExponent(sizes, medianBytes)

	// t.Logf is visible under `go test -v` (what CI uses) on both pass
	// and fail — useful for watching how close the exponent stays to 1
	// across different runners.
	t.Logf("runtime exponent=%.3f  allocation exponent=%.3f  (threshold %.1f)\n%s",
		timeExponent, spaceExponent, exponentThreshold, formatRows())

	assert.Less(t, timeExponent, exponentThreshold,
		"runtime scales as ~n^%.2f (threshold %.1f) — closer to quadratic than linear:\n%s",
		timeExponent, exponentThreshold, formatRows())
	assert.Less(t, spaceExponent, exponentThreshold,
		"allocation scales as ~n^%.2f (threshold %.1f) — closer to quadratic than linear:\n%s",
		spaceExponent, exponentThreshold, formatRows())
}
