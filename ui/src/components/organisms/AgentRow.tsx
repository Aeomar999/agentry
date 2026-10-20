import { Link } from "react-router-dom";
import { ChevronDown, ChevronRight, CircleAlert, Trash2, TriangleAlert } from "lucide-react";
import type { Policy } from "@/api/types";
import { Badge } from "@/components/atoms/Badge";
import { IconButton } from "@/components/atoms/IconButton";
import { Input } from "@/components/atoms/Input";
import { Radio } from "@/components/atoms/Radio";
import { Select } from "@/components/atoms/Select";
import { CodeViewer } from "@/components/molecules/CodeViewer";
import { ColorSwatches } from "@/components/molecules/ColorSwatches";
import { errorId, Field } from "@/components/molecules/Field";
import { MODELS } from "@/components/organisms/AgentForm";
import { COLUMN_TITLES, columnOfAgent } from "@/lib/columns";
import { type AgentField, type DraftAgent, noAgentDescription, type Problem, problemCount, shownName } from "@/lib/custom-team";
import { shippedProfiles } from "@/lib/profiles";
import { cn } from "@/lib/cn";

// One agent of the team panel (docs/design/custom-teams.md): a header that
// is always there (its name as the button that opens it, its badges, the
// team's one coordinator radio and remove), then, open, every field of the
// agent. Closed, it says its description and how many problems it has.
// On a phone the header takes two lines: the name, its badges and remove,
// then the coordinator radio and the description.
// While the team saves the fields are read-only and the rest aria-disabled.
// Editing a team, the row also says how it differs from the team as saved.

interface AgentRowProps {
  agent: DraftAgent;
  open: boolean;
  onToggle: () => void;
  problems: Problem[]; // this row's
  profileMissing: boolean; // policy.toml lacks its profile, which Agentry ships
  engines: string[];
  policy: Policy;
  saving: boolean;
  onChange: (change: Partial<DraftAgent>) => void;
  onRemove: () => void;
  edit?: RowEdit; // none in a new team
  unpicked?: boolean; // two rows coordinate, so no radio is checked until you pick one
}

// How a row of a team being edited differs from the team as saved: new or
// edited, with the name it had when renamed and whether its prompt changed,
// and its agent's name there (none for a new one), whose page has what the
// panel hasn't.
export interface RowEdit {
  mark?: "new" | "edited";
  was?: string;
  promptChanged?: boolean;
  saved?: string;
}

// fieldId is the id of a row's control, for labels, errors and the summary's links.
export const fieldId = (row: string, field: AgentField | "description" | "model") => `${row}-${field}`;
export const toggleId = (row: string) => `${row}-toggle`;

const COLUMN_HINTS: Record<string, string> = {
  "": "Checked by the team's reviewers. Its tasks show in In progress.",
  review: "Its approval is needed to finish. Its tasks show in Review.",
  ship: "Ships checked work, such as deploying. Its tasks show in Shipping.",
};

export function AgentRow(props: AgentRowProps) {
  const { agent, open, onToggle, problems, profileMissing, saving, onChange, onRemove, edit, unpicked } = props;
  const name = shownName(agent);
  const marks = [
    edit?.mark,
    edit?.was && `was ${edit.was}`,
    problems.length > 0 && problemCount(problems.length),
    profileMissing && "profile missing",
  ].filter(Boolean);
  const column = agent.coordinates ? "" : columnOfAgent(agent);
  // What this team does says it for the coordinator.
  const undescribed = !agent.coordinates && agent.description.trim() === "";
  return (
    <li className="border-b border-divider last:border-b-0">
      <div className="flex min-h-11 items-center gap-2 px-2 max-sm:grid max-sm:grid-cols-[auto_minmax(0,1fr)_auto] max-sm:gap-y-0">
        <button
          type="button"
          id={toggleId(agent.key)}
          aria-expanded={open}
          aria-label={[name, ...marks].join(", ")}
          onClick={onToggle}
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 py-2 text-left max-sm:col-span-2"
        >
          {open ? (
            <ChevronDown className="size-3.5 shrink-0 text-ink-muted" />
          ) : (
            <ChevronRight className="size-3.5 shrink-0 text-ink-muted" />
          )}
          <span
            className="size-2.5 shrink-0 rounded-full"
            style={{ background: `var(--color-agent-${agent.color || "slate"})` }}
            aria-hidden
          />
          <span className="min-w-0 truncate font-mono text-[12.5px] font-semibold text-ink-primary">{name}</span>
          {edit?.was && <span className="shrink-0 text-[12px] text-ink-muted">was {edit.was}</span>}
          {edit?.mark && <Badge>{edit.mark}</Badge>}
          {agent.coordinates && <Badge variant="brand">coordinator</Badge>}
          {column && <Badge>{COLUMN_TITLES[column]}</Badge>}
          {problems.length > 0 && (
            <span className="flex shrink-0 items-center gap-1 text-[12px] text-danger-ink">
              <CircleAlert className="size-3.5" aria-hidden />
              {problemCount(problems.length)}
            </span>
          )}
          {profileMissing && (
            <span className="flex shrink-0 items-center gap-1 text-[12px] text-warning-ink">
              <TriangleAlert className="size-3.5" aria-hidden />
              profile missing
            </span>
          )}
          {!open && (agent.description || undescribed) && (
            <span className="min-w-0 shrink-[999] truncate text-[12px] text-ink-muted max-sm:hidden">
              {agent.description || "No description"}
            </span>
          )}
        </button>
        <span className="shrink-0 max-sm:row-start-2 max-sm:pl-5.5 max-sm:[&>label]:min-h-11 max-sm:[&>label]:items-center">
          <Radio
            name="coordinator"
            checked={agent.coordinates && !unpicked}
            readOnly={saving}
            onChange={() => onChange({ coordinates: true })}
            label={
              <span className="text-[12px] text-ink-secondary">
                <span className="sr-only">Coordinator, {name}</span>
                <span aria-hidden>Coordinator</span>
              </span>
            }
          />
        </span>
        <IconButton
          icon={<Trash2 />}
          label={`Remove ${name}`}
          aria-disabled={saving || undefined}
          onClick={() => !saving && onRemove()}
          className="size-7 hover:text-danger max-sm:col-start-3 max-sm:row-start-1 max-sm:size-11"
        />
        {!open && (agent.description || undescribed) && (
          <span aria-hidden className="min-w-0 truncate text-[12px] text-ink-muted max-sm:col-span-2 max-sm:row-start-2 sm:hidden">
            {agent.description || "No description"}
          </span>
        )}
      </div>
      {open && <AgentFields {...props} />}
    </li>
  );
}

function AgentFields({ agent, problems, profileMissing, engines, policy, saving, onChange, edit }: AgentRowProps) {
  const id = (field: Parameters<typeof fieldId>[1]) => fieldId(agent.key, field);
  const errorOf = (field: AgentField) => problems.find((p) => typeof p.field === "object" && p.field.field === field)?.message;
  // What a control with a problem says about itself, beside the danger border.
  const invalid = (field: AgentField) =>
    errorOf(field) ? { "aria-invalid": true, "aria-describedby": errorId(id(field)), className: "border-danger" } : {};
  // A select takes no readOnly, so while saving it takes no change instead.
  const choose = (change: (value: string) => Partial<DraftAgent>) => (event: { target: { value: string } }) =>
    !saving && onChange(change(event.target.value));
  const lines = agent.prompt === "" ? 0 : agent.prompt.split("\n").length;
  const lineCount = lines === 1 ? "1 line" : lines > 1 ? `${lines} lines` : undefined;
  // Agentry writes its coordinator prompt only for an agent it adds, so
  // the coordinator a team has can't go without one.
  const takesDefault = agent.coordinates && !edit?.saved;
  const defaultPrompt = takesDefault && agent.prompt.trim() === "";
  const shipped = shippedProfiles(policy);
  const profileKnown = agent.profile === "" || policy.profiles.some((p) => p.name === agent.profile);
  const renamed = edit?.saved && agent.name.trim() !== edit.saved ? edit.saved : undefined;

  return (
    <div className="flex flex-col gap-4 border-t border-divider bg-surface-0 px-4 py-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          label="Name"
          htmlFor={id("name")}
          hint={
            renamed
              ? `Renames ${renamed}: it keeps its prompt, settings and what it knows, and its tasks go on with the new name.`
              : "Lowercase letters, digits and dashes."
          }
          error={errorOf("name")}
        >
          <Input
            id={id("name")}
            mono
            value={agent.name}
            readOnly={saving}
            onChange={(e) => onChange({ name: e.target.value.toLowerCase() })}
            {...invalid("name")}
          />
        </Field>
        <Field
          label="Description"
          htmlFor={id("description")}
          hint={
            agent.coordinates ? (
              "What this team does. Other coordinators read it to choose what to send the team."
            ) : agent.description.trim() === "" ? (
              <span className="text-[12px]">{noAgentDescription}</span>
            ) : (
              "One line. The coordinator reads it to choose who works next."
            )
          }
        >
          <Input
            id={id("description")}
            value={agent.description}
            readOnly={saving}
            onChange={(e) => onChange({ description: e.target.value })}
          />
        </Field>
      </div>

      <Field
        label="Prompt"
        labelId={`${id("prompt")}-label`}
        aside={lineCount && edit?.promptChanged ? `${lineCount} · changed` : lineCount}
        error={errorOf("prompt")}
        hint={
          <>
            {defaultPrompt
              ? "Empty: it uses Agentry's coordinator prompt for a team of these agents."
              : "Its standing instructions, at the start of every turn. Tab moves to the next field."}
            {edit?.saved && (
              <span className="block">
                {renamed
                  ? `Renaming doesn't change its prompt: it may still say "${renamed}".`
                  : "Renaming an agent doesn't change its prompt."}
              </span>
            )}
          </>
        }
      >
        <div
          id={id("prompt")}
          role="group"
          aria-labelledby={`${id("prompt")}-label`}
          aria-describedby={errorOf("prompt") ? errorId(`${id("prompt")}-label`) : undefined}
          className={cn(errorOf("prompt") && "rounded-md border border-danger")}
        >
          <CodeViewer
            text={agent.prompt}
            language="markdown"
            minHeight={180} // 8 lines
            maxHeight={500} // 24 lines
            tabFocusMode
            alwaysConsumeMouseWheel={false}
            readOnly={saving}
            placeholder={takesDefault ? "Empty: it uses Agentry's coordinator prompt for a team of these agents." : undefined}
            onChange={(prompt) => onChange({ prompt })}
          />
        </div>
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          label="Colour"
          labelId={id("color")}
          error={errorOf("color")}
          hint={
            agent.color
              ? `${agent.color[0].toUpperCase()}${agent.color.slice(1)}. Agents can share a colour.`
              : "Agents can share a colour."
          }
        >
          <ColorSwatches value={agent.color} labelledBy={id("color")} disabled={saving} onChange={(color) => onChange({ color })} />
        </Field>
        <Field
          label="Its work counts as"
          htmlFor={id("column")}
          error={errorOf("column")}
          hint={agent.coordinates ? "Not used: a coordinator hands the work out." : COLUMN_HINTS[agent.column]}
        >
          <Select
            id={id("column")}
            value={agent.coordinates ? "" : agent.column}
            disabled={agent.coordinates}
            aria-disabled={saving || undefined}
            onChange={choose((column) => ({ column: column as DraftAgent["column"] }))}
            {...invalid("column")}
          >
            <option value="">Building</option>
            <option value="review">Review</option>
            <option value="ship">Shipping</option>
          </Select>
        </Field>

        <div className="flex flex-col gap-4">
          <Field label="Engine" htmlFor={id("engine")} error={errorOf("engine")}>
            <Select
              id={id("engine")}
              value={agent.engine}
              aria-disabled={saving || undefined}
              onChange={choose((engine) => ({ engine }))}
              {...invalid("engine")}
            >
              {!engines.includes(agent.engine) && <option value={agent.engine}>{agent.engine || "Pick an engine"}</option>}
              {engines.map((engine) => (
                <option key={engine} value={engine}>
                  {engine}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Model" htmlFor={id("model")} hint="Empty for the engine's default.">
            <Input
              id={id("model")}
              mono
              list={`${agent.key}-models`}
              value={agent.model}
              readOnly={saving}
              onChange={(e) => onChange({ model: e.target.value })}
            />
            <datalist id={`${agent.key}-models`}>
              {(MODELS[agent.engine] ?? []).map((model) => (
                <option key={model} value={model} />
              ))}
            </datalist>
          </Field>
        </div>
        <Field
          label="Policy profile"
          htmlFor={id("profile")}
          error={errorOf("profile")}
          hint={
            profileMissing
              ? `${agent.profile} (missing from policy.toml): every call it makes is denied until it's back. ${edit ? "Save changes" : "Create team"} offers to add it, or pick another.`
              : "What it may do without asking you."
          }
        >
          <Select
            id={id("profile")}
            value={agent.profile}
            aria-disabled={saving || undefined}
            onChange={choose((profile) => ({ profile }))}
            className={profileMissing ? "border-warning" : undefined}
            {...invalid("profile")}
          >
            <option value="">default ({policy.defaultProfile})</option>
            {!profileKnown && (
              <option value={agent.profile}>
                {agent.profile} ({shipped.has(agent.profile) ? "missing from" : "not in"} policy.toml)
              </option>
            )}
            {policy.profiles.map((profile) => (
              <option key={profile.name} value={profile.name}>
                {profile.name}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <PageLine saved={edit ? (edit.saved ?? "") : undefined} />
    </div>
  );
}

// PageLine says where the rest of an agent is: its own page, once it has
// one. saved is its name as saved when editing a team, "" for a new agent.
function PageLine({ saved }: { saved?: string }) {
  if (saved === undefined)
    return <p className="text-[12px] text-ink-muted">Knowledge and where it runs: on its own page, once the team is created.</p>;
  if (saved === "") return <p className="text-[12px] text-ink-muted">Knowledge and where it runs: on its own page, once you save.</p>;
  return (
    <p className="text-[12px] text-ink-muted">
      Knowledge, where it runs and a taller prompt editor:{" "}
      <Link to={`/agents/${encodeURIComponent(saved)}`} className="text-ink-primary underline hover:no-underline">
        Open {saved}&apos;s page
      </Link>
    </p>
  );
}
