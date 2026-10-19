import { Lightbulb } from "lucide-react";
import type { FeatureUI } from "..";
import { IdeasPage } from "./IdeasPage";

export const ideas: FeatureUI = {
  name: "ideas",
  routes: [{ path: "/ideas", element: <IdeasPage /> }],
  nav: [{ section: "work", to: "/ideas", label: "Ideas", icon: <Lightbulb /> }],
  commands: (navigate) => [{ id: "go-ideas", group: "Go to", label: "Ideas", icon: <Lightbulb />, run: () => navigate("/ideas") }],
};
