import { useRef } from "react";
import { useBreadcrumb } from "@/lib/breadcrumb";
import { PageHeader } from "@/components/molecules/PageHeader";
import { Panel } from "@/components/molecules/Panel";
import { Banner } from "@/components/molecules/Banner";
import { type FeatureUI, FEATURES, hiddenFeatures, unknownLine, useFeatures } from "@/features";

// Settings lists the features this build has that are off, so one that
// doesn't show can still be found: each with the server's why line and its
// own button that turns it on. Below them, the events the server skipped
// because this build lacks the feature or the type that wrote them, so a
// downgrade is explained rather than silent.
//
// A feature listed as off stays listed until the user leaves the page: its
// turn-on button may still be at work when the manifest says it is on.

export function SettingsPage() {
  useBreadcrumb({ label: "Settings" });
  const features = useFeatures();
  const listed = useRef(new Set<string>());
  hiddenFeatures(features).forEach((feature) => listed.current.add(feature.name));
  const off = FEATURES.filter((feature) => listed.current.has(feature.name));

  return (
    <>
      <PageHeader title="Settings" meta="Features that are off, and how to turn them on." />
      <div className="flex max-w-5xl flex-col gap-6 p-6">
        {features.error && <Banner title="Couldn't read the features">{features.error}</Banner>}
        {features.loaded && off.length === 0 && <p className="text-[13px] text-ink-muted">Every feature is on.</p>}
        {off.length > 0 && (
          <Panel title="Features that are off" flush>
            <ul className="divide-y divide-divider">
              {off.map((feature) => (
                <FeatureOff key={feature.name} feature={feature} why={features.line(feature.name)?.why} />
              ))}
            </ul>
          </Panel>
        )}
        {features.unknown.length > 0 && (
          <ul className="flex flex-col gap-1 text-[12px] text-ink-muted">
            {features.unknown.map((unknown) => (
              <li key={unknown.prefix ?? unknown.type}>{unknownLine(unknown)}</li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}

// FeatureOff is one feature that is off: its name, why, and its button.
function FeatureOff({ feature, why }: { feature: FeatureUI; why?: string }) {
  const TurnOn = feature.turnOn;
  return (
    <li className="flex flex-col gap-3 px-4 py-3">
      <div className="flex flex-col gap-0.5">
        <span className="text-[13px] font-medium text-ink-primary">{feature.label}</span>
        {why && <span className="text-[12px] text-ink-muted">{why}</span>}
      </div>
      {TurnOn && <TurnOn />}
    </li>
  );
}
