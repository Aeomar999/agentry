import { ReactNode, useMemo } from "react";
import type { NavigateFunction, RouteObject } from "react-router-dom";
import type { PaletteItem } from "@/components/organisms/CommandPalette";
import type { BoardColumn, FeaturesReply, TaskSummary } from "@/api/types";
import { useResource } from "@/live/use-resource";
import { COLUMN_TITLES } from "@/lib/columns";
import { ideas } from "./ideas";

// A feature's UI is one folder here whose index exports a FeatureUI. The
// router, the sidebar and Ctrl K go through FEATURES, and GET /api/features
// says which of them show: the page decides how a feature looks, the server
// whether it shows.

export interface FeatureUI {
  name: string; // the server's feature name in GET /api/features
  routes?: RouteObject[]; // always mounted, so a bookmark works when the feature is off
  nav?: { section: "work" | "setup"; to: string; label: string; icon: ReactNode }[];
  commands?: (navigate: NavigateFunction) => PaletteItem[]; // Ctrl K, under "Go to" or "Commands"
}

export const FEATURES: FeatureUI[] = [ideas];

export interface Features {
  visible: (name: string) => boolean;
  badge: (name: string) => number; // 0 for none
  columns: BoardColumn[]; // the board's, in order
}

// useFeatures reads GET /api/features, again whenever the log moves, and
// says which features show. A name only one side knows is ignored, and while
// the manifest is loading, or has never loaded, no feature shows.
export function useFeatures(): Features {
  const { data } = useResource<FeaturesReply>("/api/features");
  return useMemo(() => {
    const lines = new Map((data?.features ?? []).map((line) => [line.name, line]));
    return {
      visible: (name) => lines.get(name)?.visible ?? false,
      badge: (name) => lines.get(name)?.badge ?? 0,
      columns: boardColumns(data),
    };
  }, [data]);
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
