import { send } from "@/api/client";
import { Read } from "./resource-cache";

// readPath reads one API path, with the log position the server read it at,
// or none when the reply doesn't say (a proxy dropped the header).
export async function readPath(path: string): Promise<Read> {
  const response = await send(path, { method: "GET" });
  return { data: await response.json(), position: positionOf(response.headers.get("X-Agentry-Position")) };
}

function positionOf(header: string | null): number | undefined {
  return header !== null && /^\d+$/.test(header) ? Number(header) : undefined;
}
