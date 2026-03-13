import { markdownResponse, staticMarkdownPaths } from "../content";

export function generateStaticParams() {
  return staticMarkdownPaths();
}

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params;
  return markdownResponse(path.join("/"));
}
