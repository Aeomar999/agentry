import { Lightbulb } from "lucide-react";
import type { FeatureUI } from "..";
import { IdeasPage } from "./IdeasPage";
import { TurnOnIdeas } from "./AskVisionaries";

export const ideas: FeatureUI = {
  name: "ideas",
  label: "Ideas",
  routes: [{ path: "/ideas", element: <IdeasPage /> }],
  nav: [{ section: "work", to: "/ideas", label: "Ideas", icon: <Lightbulb /> }],
  commands: (navigate) => [{ id: "go-ideas", group: "Go to", label: "Ideas", icon: <Lightbulb />, run: () => navigate("/ideas") }],
  turnOn: TurnOnIdeas,
};
