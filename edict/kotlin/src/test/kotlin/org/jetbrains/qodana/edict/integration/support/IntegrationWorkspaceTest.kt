package org.jetbrains.qodana.edict.integration.support

import org.jetbrains.qodana.edict.support.afterSource
import org.jetbrains.qodana.edict.support.fixturePath
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class IntegrationWorkspaceTest {
    @TempDir
    lateinit var directory: Path
    @BeforeEach
    fun canonicalTemporaryDirectory() {
        directory = directory.toRealPath()
    }

    @Test
    fun `each run clones committed history and clears only its own previous artifacts`() {
        val source = gitFixture(directory.resolve("source"))
        val revision = source.resolve("HEAD")
        Files.writeString(source.root.resolve(fixturePath), "dirty source checkout")
        Files.writeString(source.root.resolve("untracked"), "do not copy or delete")
        val out = directory.resolve("out")
        val sibling = out.resolve("AnotherTest/keep")
        Files.createDirectories(sibling.parent)
        Files.writeString(sibling, "keep")
        fun open() = IntegrationWorkspace.open(out, "FixtureTest", "one commit", source.root.toString(), revision, ".")
        val first = open().use { workspace ->
            assertEquals(afterSource, Files.readString(workspace.repository.root.resolve(fixturePath)))
            assertFalse(Files.exists(workspace.repository.root.resolve("untracked")))
            assertFalse(Files.exists(workspace.repository.root.resolve(".git/objects/info/alternates")))
            Files.createDirectories(workspace.output.resolve("log"))
            Files.writeString(workspace.output.resolve("log/previous.log"), "old")
            Files.writeString(workspace.repository.root.resolve(fixturePath), "modified clone")
            assertFailsWith<IllegalStateException> { open() }
            assertEquals(
                "old",
                Files.readString(workspace.output.resolve("log/previous.log")),
                "Concurrent attempt must not delete the active run"
            )
            workspace.output
        }
        assertTrue(Files.exists(first.resolve("log/previous.log")), "Artifacts survive completion")
        open().use { workspace ->
            assertEquals(first, workspace.output)
            assertFalse(Files.exists(workspace.output.resolve("log/previous.log")))
            assertEquals(afterSource, Files.readString(workspace.repository.root.resolve(fixturePath)))
            assertEquals("", workspace.repository.git("status", "--porcelain"))
            workspace.assertCheckoutUnchanged()
        }
        assertEquals("keep", Files.readString(sibling))
        assertEquals("dirty source checkout", Files.readString(source.root.resolve(fixturePath)))
        assertEquals("do not copy or delete", Files.readString(source.root.resolve("untracked")))
    }

    @Test
    fun `cleanup refuses redirected output without deleting the target`() {
        val target = directory.resolve("target")
        Files.createDirectory(target)
        Files.writeString(target.resolve("keep"), "keep")
        val out = directory.resolve("redirected")
        Files.createSymbolicLink(out, target)
        assertFailsWith<IllegalArgumentException> {
            IntegrationWorkspace.open(
                out,
                "FixtureTest",
                "one commit",
                "unused"
            )
        }
        assertEquals("keep", Files.readString(target.resolve("keep")))
    }

    @Test
    fun `committed initial state is opt in and copied without changing fixture files`() {
        val source = gitFixture(directory.resolve("source"))
        val fixtureState = "scenario/.edict"
        val relativeSignal = "clusters/prepared/signals/s-seed.json"
        val committed = source.root.resolve(fixtureState).resolve(relativeSignal)
        Files.createDirectories(committed.parent)
        Files.writeString(committed, "committed seed")
        source.git("add", "scenario")
        source.git("commit", "-qm", "Prepare fixture state")
        val revision = source.resolve("HEAD")
        Files.writeString(committed, "uncommitted local seed")
        val out = directory.resolve("out")
        IntegrationWorkspace.open(out, "FixtureTest", "without seed", source.root.toString(), revision, "scenario").use { workspace ->
            assertFalse(Files.exists(workspace.state), "Legacy fixture state must not load implicitly")
            workspace.assertCheckoutUnchanged()
        }
        fun open() = IntegrationWorkspace.open(
            out, "FixtureTest", "with seed", source.root.toString(), revision, "scenario", fixtureState,
        )
        open().use { workspace ->
            assertEquals("committed seed", Files.readString(workspace.state.resolve(relativeSignal)))
            Files.writeString(workspace.state.resolve(relativeSignal), "enriched disposable state")
            assertEquals("committed seed", Files.readString(workspace.project.resolve(".edict").resolve(relativeSignal)))
            workspace.assertCheckoutUnchanged()
        }
        open().use { workspace ->
            assertEquals("committed seed", Files.readString(workspace.state.resolve(relativeSignal)))
            workspace.assertCheckoutUnchanged()
        }
        assertEquals("uncommitted local seed", Files.readString(committed))
    }

    @Test
    fun `initial state rejects paths outside the checkout and redirected files`() {
        val source = gitFixture(directory.resolve("source"))
        val external = directory.resolve("external")
        Files.createDirectory(external)
        Files.writeString(external.resolve("keep"), "keep")
        val fixtureState = source.root.resolve("scenario/.edict")
        Files.createDirectories(fixtureState)
        Files.createSymbolicLink(fixtureState.resolve("redirected"), external.resolve("keep"))
        source.git("add", "scenario")
        source.git("commit", "-qm", "Add redirected fixture")
        for ((index, initialState) in listOf(external.toString(), "../external", ".edict", "scenario/.edict").withIndex()) {
            assertFailsWith<IllegalArgumentException> {
                IntegrationWorkspace.open(
                    directory.resolve("out"), "FixtureTest", "reject $index", source.root.toString(), source.resolve("HEAD"),
                    "scenario", initialState,
                )
            }
        }
        assertEquals("keep", Files.readString(external.resolve("keep")))
    }
}
