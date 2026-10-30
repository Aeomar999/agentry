import { ComponentType, ReactNode, useMemo } from "react";
import type { NavigateFunction, RouteObject } from "react-router-dom";
import type { PaletteItem } from "@/shell/CommandPalette";
import type { BoardColumn, FeatureLine, FeaturesReply, TaskSummary, UnknownEvents } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { COLUMN_TITLES } from "@/lib/columns";
import type { ExtensionStreams } from "./streams";
import { ideas } from "./ideas";

// An extension's UI is one folder here whose index exports an ExtensionUI. The
// router, the sidebar and Ctrl K go through EXTENSIONS, and GET /api/features
// says which of them show: the page decides how an extension looks, the server
// whether it shows. Settings lists the extensions that don't show, each with
// its turnOn.

export interface ExtensionUI {
  name: string; // the server's extension name in GET /api/features
  label: string; // what the page calls it
  routes?: RouteObject[]; // always mounted, so a bookmark works when the extension is off
  nav?: { section: "work" | "setup"; to: string; label: string; icon: ReactNode }[];
  commands?: (navigate: NavigateFunction) => PaletteItem[]; // Ctrl K, under "Go to" or "Commands"
  turnOn?: ComponentType; // the extension's own button that turns it on, on Settings while it is off
  streams?: ExtensionStreams; // the streams it owns, linked from a task they made
}

export const EXTENSIONS: ExtensionUI[] = [ideas];

export { extensionLink } from "./streams";

export interface Extensions {
  loaded: boolean; // the manifest is read
  error: string; // why it couldn't be read
  line: (name: string) => FeatureLine | undefined; // undefined for an extension the server doesn't list
  visible: (name: string) => boolean;
  badge: (name: string) => number; // 0 for none
  columns: BoardColumn[]; // the board's, in order
  unknown: UnknownEvents[]; // events this build skipped or didn't understand
}

// useExtensions reads GET /api/features, again whenever the log moves, and
// says which extensions show.
export function useExtensions(): Extensions {
  const { data, error } = useResource<FeaturesReply>("/api/features");
  return useMemo(() => extensionsOf(data, error), [data, error]);
}

// extensionsOf reads a manifest. A name only one side knows is ignored, and
// while the manifest is loading, or has never loaded, no extension shows.
export function extensionsOf(manifest: FeaturesReply | undefined, error = ""): Extensions {
  const lines = new Map((manifest?.features ?? []).map((line) => [line.name, line]));
  return {
    loaded: manifest !== undefined,
    error,
    line: (name) => lines.get(name),
    visible: (name) => lines.get(name)?.visible ?? false,
    badge: (name) => lines.get(name)?.badge ?? 0,
    columns: boardColumns(manifest),
    unknown: manifest?.unknown ?? [],
  };
}

const EVERY_COLUMN = Object.keys(COLUMN_TITLES) as BoardColumn[];

// boardColumns are the columns the manifest lists, those this page knows.
// Before it loads, Shipping waits like any extension; a server too old to
// list them gets every column, as it had before.
export function boardColumns(reply: FeaturesReply | undefined): BoardColumn[] {
  if (!reply) return EVERY_COLUMN.filter((column) => column !== "shipping");
  if (!reply.board) return EVERY_COLUMN;
  return reply.board.columns.filter((column) => column in COLUMN_TITLES);
}

// withTheirCards adds back, in the board's order, any column a task is in:
// a manifest still loading, or listed before a task moved, never hides a
// card.
export function withTheirCards(columns: BoardColumn[], tasks: TaskSummary[]): BoardColumn[] {
  return EVERY_COLUMN.filter((column) => columns.includes(column) || tasks.some((task) => task.column === column));
}

// visibleExtensions is the EXTENSIONS the manifest says show, in EXTENSIONS' order.
export function visibleExtensions(extensions: Extensions): ExtensionUI[] {
  return EXTENSIONS.filter((extension) => extensions.visible(extension.name));
}

// hiddenExtensions is the EXTENSIONS the manifest lists but doesn't show: those
// installed and off, in the list's order.
export function hiddenExtensions(extensions: Extensions, list = EXTENSIONS): ExtensionUI[] {
  return list.filter((extension) => extensions.line(extension.name)?.visible === false);
}

// unknownLine words one entry of the unknown tally.
export function unknownLine(unknown: UnknownEvents): string {
  const events = unknown.events === 1 ? "1 event" : `${unknown.events} events`;
  if (unknown.prefix) return `${events} from a feature this build doesn't have: ${unknown.prefix}`;
  return `${events} of a type this build doesn't know: ${unknown.type}`;
}
