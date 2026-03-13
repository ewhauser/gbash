import {
  FILE_README,
  FILE_GO_MOD,
  FILE_AGENTS_MD,
  FILE_VERSION_GO,
  FILE_WEBSITE_README,
  FILE_WTF_IS_THIS,
} from "../components/terminal-content";

export const FILES: Record<string, string> = Object.assign(Object.create(null), {
  "README.md": FILE_README,
  "go.mod": FILE_GO_MOD,
  "AGENTS.md": FILE_AGENTS_MD,
  "cmd/gbash/version.go": FILE_VERSION_GO,
  "examples/website/README.md": FILE_WEBSITE_README,
  "wtf-is-this.md": FILE_WTF_IS_THIS,
});

export function staticMarkdownPaths() {
  return Object.keys(FILES).map((file) => ({ path: file.split("/") }));
}

export function markdownResponse(filePath: string) {
  const content = FILES[filePath];

  if (!content) {
    return new Response(
      `File not found: ${filePath}\n\nAvailable files:\n${Object.keys(FILES).join("\n")}`,
      {
        status: 404,
        headers: { "Content-Type": "text/plain; charset=utf-8" },
      }
    );
  }

  const contentType = filePath.endsWith(".json")
    ? "application/json"
    : filePath.endsWith(".md")
      ? "text/markdown; charset=utf-8"
      : "text/plain; charset=utf-8";

  return new Response(content, {
    headers: { "Content-Type": contentType },
  });
}
