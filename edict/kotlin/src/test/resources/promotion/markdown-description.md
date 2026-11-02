## Description

Adds the generated Kotlin inspection `rule` at `inspections/rule.inspection.kts`.

Representative evidence: Do not use \[unsafe\](https://wrong.test) &lt;markup&gt; or \`ticks\` across lines

## Strong signals

Showing referenced STRONG POSITIVE signals from 5 of 5 distinct source files, one per file. POSITIVE marks problematic code.

1. **POSITIVE** — Do not use \[unsafe\](https://wrong.test) &lt;markup&gt; or \`ticks\` across lines  
   Source: [PR #1: Correction 0](<https://github.com/source/project/pull/1#discussion_r1>)  
   Evidence: Example0.kt at `c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00`; signal `signal-00`.

2. **POSITIVE** — positive 1  
   Source: [PR #2: Fix \*bold\* \_under\_ \#1 &amp; co\\](<https://github.com/source/project/pull/2#discussion_r2>)  
   Evidence: Example1.kt at `c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00`; signal `signal-01`.

3. **POSITIVE** — positive 2  
   Source: [PR #3: Correction 2](<https://github.com/source/project/pull/3#discussion_r3>)  
   Evidence: Example2.kt at `c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00`; signal `signal-02`.

4. **POSITIVE** — positive 3  
   Source: [PR #4: Correction 3](<https://github.com/source/project/pull/4#discussion_r4>)  
   Evidence: Example3.kt at `c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00`; signal `signal-03`.

5. **POSITIVE** — positive 4  
   Source: [PR #5: Correction 4](<https://github.com/source/project/pull/5#discussion_r5>)  
   Evidence: Example4.kt at `c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00`; signal `signal-04`.

Inspection SHA-256: `3b1957f394d1b896c8064464feca7a2fcdcacad82e22ee7413eca63e5e7cc596`.
