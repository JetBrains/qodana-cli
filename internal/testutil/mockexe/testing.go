// MockExeT and the sentinel types in this file provide a goroutine-safe
// testing.TB for use inside mockexe handlers.
//
// Handlers run on a background goroutine (the TCP server's), not on the
// test goroutine. The standard testing.T's FailNow/Fatal/Fatalf call
// runtime.Goexit, which would kill the server goroutine instead of the
// test. MockExeT replaces those methods with panic-based control flow
// so that require/assert work correctly inside handlers.
//
// Always use ctx.T (the MockExeT) inside a handler, never the outer t
// captured by the closure — the outer t is the real *testing.T and its
// FailNow would terminate the wrong goroutine.
package mockexe

import "testing"

// failNowSentinel is panicked by MockExeT when FailNow/Fatal/Fatalf is called.
// Recovered in callHandlerSafe → exit code 1.
type failNowSentinel struct{}

// skipNowSentinel is panicked by MockExeT when SkipNow/Skip/Skipf is called.
// Recovered in callHandlerSafe → exit code 0.
type skipNowSentinel struct{}

// MockExeT wraps testing.TB so that methods calling runtime.Goexit are
// replaced with panic-based control flow. This makes require/assert/t.Fatalf
// safe to use from a handler running on a background goroutine.
//
// Embedding testing.TB satisfies the interface (including unexported methods)
// and delegates all safe methods automatically.
type MockExeT struct{ testing.TB }

func (s *MockExeT) FailNow()                         { s.Fail(); panic(failNowSentinel{}) }
func (s *MockExeT) Fatal(args ...any)                  { s.Error(args...); panic(failNowSentinel{}) }
func (s *MockExeT) Fatalf(format string, args ...any)  { s.Errorf(format, args...); panic(failNowSentinel{}) }
func (s *MockExeT) SkipNow()                           { panic(skipNowSentinel{}) }
func (s *MockExeT) Skip(args ...any)                   { s.Log(args...); panic(skipNowSentinel{}) }
func (s *MockExeT) Skipf(format string, args ...any)   { s.Logf(format, args...); panic(skipNowSentinel{}) }
