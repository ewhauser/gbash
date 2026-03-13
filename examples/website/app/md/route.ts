import { markdownResponse } from "./content";

export async function GET() {
  return markdownResponse("README.md");
}
