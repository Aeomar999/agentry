import { ComponentType, ReactNode, useMemo } from "react";
import type { NavigateFunction, RouteObject } from "react-router-dom";
import type { PaletteItem } from "@/components/organisms/CommandPalette";
import type { BoardColumn, FeatureLine, FeaturesReply, TaskSummary, UnknownEvents } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { COLUMN_TITLES } from "@/lib/columns";
import type { FeatureStreams } from "./streams";
import { ideas } from "./ideas";

// A feature's UI is one folder here whose index exports a FeatureUI. The
// router, the sidebar and Ctrl K go through FEATURES, and GET /api/features
// says which of them show: the page decides how a feature looks, the server
// whether it shows. Settings lists the features that don't show, each with
// its turnOn.

export interface FeatureUI {
  name: string; // the server's feature name in GET /api/features
  label: string; // what the page calls it
  routes?: RouteObject[]; // always mounted, so a bookmark works when the feature is off
  nav?: { section: "work" | "setup"; to: string; label: string; icon: ReactNode }[];
  commands?: (navigate: NavigateFunction) => PaletteItem[]; // Ctrl K, under "Go to" or "Commands"
  turnOn?: ComponentType; // the feature's own button that turns it on, on Settings while it is off
  streams?: FeatureStreams; // the streams it owns, linked from a task they made
}

export const FEATURES: FeatureUI[] = [ideas];

export { featureLink } from "./streams";

export interface Features {
  loaded: boolean; // the manifest is read
  error: string; // why it couldn't be read
  line: (name: string) => FeatureLine | undefined; // undefined for a feature the server doesn't list
  visible: (name: string) => boolean;
  badge: (name: string) => number; // 0 for none
  columns: BoardColumn[]; // the board's, in order
  unknown: UnknownEvents[]; // events this build skipped or didn't understand
}

// useFeatures reads GET /api/features, again whenever the log moves, and
// says which features show.
export function useFeatures(): Features {
  const { data, error } = useResource<FeaturesReply>("/api/features");
  return useMemo(() => featuresOf(data, error), [data, error]);
}

// featuresOf reads a manifest. A name only one side knows is ignored, and
// while the manifest is loading, or has never loaded, no feature shows.
export function featuresOf(manifest: FeaturesReply | undefined, error = ""): Features {
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
// Before it loads, Shipping waits like any feature; a server too old to
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

// visibleFeatures is the FEATURES the manifest says show, in FEATURES' order.
export function visibleFeatures(features: Features): FeatureUI[] {
  return FEATURES.filter((feature) => features.visible(feature.name));
}

// hiddenFeatures is the FEATURES the manifest lists but doesn't show: those
// installed and off, in the list's order.
export function hiddenFeatures(features: Features, list = FEATURES): FeatureUI[] {
  return list.filter((feature) => features.line(feature.name)?.visible === false);
}

// unknownLine words one entry of the unknown tally.
export function unknownLine(unknown: UnknownEvents): string {
  const events = unknown.events === 1 ? "1 event" : `${unknown.events} events`;
  if (unknown.prefix) return `${events} from a feature this build doesn't have: ${unknown.prefix}`;
  return `${events} of a type this build doesn't know: ${unknown.type}`;
}
