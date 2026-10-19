import { ReactNode, useMemo } from "react";
import type { NavigateFunction, RouteObject } from "react-router-dom";
import type { PaletteItem } from "@/components/organisms/CommandPalette";
import type { FeatureLine } from "@/api/types";
import { useResource } from "@/live/use-resource";
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
}

// useFeatures reads GET /api/features, again whenever the log moves, and
// says which features show. A name only one side knows is ignored, and while
// the manifest is loading or failed to load, no feature shows.
export function useFeatures(): Features {
  const { data, error } = useResource<{ features: FeatureLine[] }>("/api/features");
  return useMemo(() => {
    const lines = new Map((error ? [] : (data?.features ?? [])).map((line) => [line.name, line]));
    return {
      visible: (name) => lines.get(name)?.visible ?? false,
      badge: (name) => lines.get(name)?.badge ?? 0,
    };
  }, [data, error]);
}

// visibleFeatures is the FEATURES the manifest says show, in FEATURES' order.
export function visibleFeatures(features: Features): FeatureUI[] {
  return FEATURES.filter((feature) => features.visible(feature.name));
}
