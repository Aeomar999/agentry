import { createContext, useContext, useEffect } from "react";
import type { Crumb } from "@/components/molecules/Breadcrumbs";

// A page names where it is with useBreadcrumb, and AdminShell, which provides
// CrumbContext, shows it in the top bar and the tab title. It lives apart from
// AdminShell so a page doesn't import the shell that imports it.

export const CrumbContext = createContext<(crumbs: Crumb[]) => void>(() => {});

export function useBreadcrumb(...crumbs: Crumb[]): void {
  const setCrumbs = useContext(CrumbContext);
  const key = JSON.stringify(crumbs);
  useEffect(() => setCrumbs(JSON.parse(key)), [key, setCrumbs]);
}
