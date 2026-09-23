package org.jetbrains.qodana.edict

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.*

class IntegrationWorkspaceTest {
    @TempDir lateinit var directory: Path
    @BeforeEach fun canonicalTemporaryDirectory() { directory = directory.toRealPath() }

    @Test fun `each run clones committed history and clears only its own previous artifacts`() {
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
            assertEquals("old", Files.readString(workspace.output.resolve("log/previous.log")), "Concurrent attempt must not delete the active run")
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

    @Test fun `cleanup refuses redirected output without deleting the target`() {
        val target = directory.resolve("target")
        Files.createDirectory(target)
        Files.writeString(target.resolve("keep"), "keep")
        val out = directory.resolve("redirected")
        Files.createSymbolicLink(out, target)
        assertFailsWith<IllegalArgumentException> { IntegrationWorkspace.open(out, "FixtureTest", "one commit", "unused") }
        assertEquals("keep", Files.readString(target.resolve("keep")))
    }
}
