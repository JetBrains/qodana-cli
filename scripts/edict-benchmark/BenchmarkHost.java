// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;
import org.jetbrains.qodana.edict.mcp.McpServer;
import org.jetbrains.qodana.edict.reviews.ReviewClient;
import org.jetbrains.qodana.edict.runtime.CodexRunner;
import org.jetbrains.qodana.edict.store.Store;

/** Small host adapter; skills, authorization, orchestration and logging come from the published Kotlin JAR. */
public class BenchmarkHost {
    public static void main(String[] args) throws Exception {
        if (args.length != 6) throw new IllegalArgumentException("project output inspection-url model minutes preflight");
        Path project = Path.of(args[0]).toRealPath();
        Path output = Path.of(args[1]).toRealPath();
        try (Store store = new Store(output.resolve("state"))) {
            McpServer server = new McpServer(store, new ReviewClient(), output.resolve("log/edict"));
            try (var transport = server.serveHttp(0)) {
                CodexRunner runner = new CodexRunner(output, project, store.getRoot(), transport.getUrl(),
                    "codex", args[3], server.getAgents(), Map.of("inspection", args[2]));
                runner.prepare();
                Path config = runner.getHome().resolve("config.toml");
                String permissions = Files.readString(config);
                String denied = "";
                for (Path path : new Path[] {project.resolve("benchmark"), project.resolve(".edict"), output.resolve("inputs.json")}) {
                    denied += "\"" + path.toString().replace("\\", "\\\\").replace("\"", "\\\"") + "\" = \"deny\"\n";
                }
                permissions = permissions.replace("[permissions.edict-test.workspace_roots]", denied + "[permissions.edict-test.workspace_roots]");
                permissions = permissions.replace("tool_timeout_sec = 300", "tool_timeout_sec = 1800");
                Files.writeString(config, permissions);
                runner.verifySandbox();
                System.out.println("Kotlin managed skills installed; state-write isolation verified; both MCP servers ready.");
                if (Boolean.parseBoolean(args[5])) return;
                String result = runner.run(Files.readString(output.resolve("prompt.txt")), Long.parseLong(args[4]));
                Files.writeString(output.resolve("last-message.txt"), store.redact(result));
                var plan = store.plan();
                if (plan == null || plan.getTasks().isEmpty() ||
                    plan.getTasks().stream().anyMatch(task -> !task.getStatus().equals("completed"))) {
                    throw new IllegalStateException("Managed benchmark plan has missing, failed or unfinished tasks");
                }
            }
        }
    }
}
