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
import type { AgentField, DraftAgent, Problem } from "@/lib/custom-team";
import { shippedProfiles } from "@/lib/profiles";
import { cn } from "@/lib/cn";

// One agent of the team panel (docs/design/custom-teams.md): a header that
// is always there (its name as the button that opens it, its badges, the
// team's one coordinator radio and remove), then, open, every field of the
// agent. Closed, it says its description and how many problems it has.
// While the team saves the fields are read-only and the rest aria-disabled.

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
  const { agent, open, onToggle, problems, profileMissing, saving, onChange, onRemove } = props;
  const name = agent.name.trim() || "New agent";
  const marks = [problems.length > 0 && problemCount(problems.length), profileMissing && "profile missing"].filter(Boolean);
  const column = agent.coordinates ? "" : columnOfAgent(agent);
  return (
    <li className="border-b border-divider last:border-b-0">
      <div className="flex min-h-11 items-center gap-2 px-2">
        <button
          type="button"
          id={toggleId(agent.key)}
          aria-expanded={open}
          aria-label={[name, ...marks].join(", ")}
          onClick={onToggle}
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 py-2 text-left"
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
          <span className="truncate font-mono text-[12.5px] font-semibold text-ink-primary">{name}</span>
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
          {!open && agent.description && (
            <span className="min-w-0 truncate text-[12px] text-ink-muted max-sm:hidden">{agent.description}</span>
          )}
        </button>
        <span className="shrink-0">
          <Radio
            name="coordinator"
            checked={agent.coordinates}
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
          className="size-7 hover:text-danger"
        />
      </div>
      {open && <AgentFields {...props} />}
    </li>
  );
}

const problemCount = (n: number) => (n === 1 ? "1 problem" : `${n} problems`);

function AgentFields({ agent, problems, profileMissing, engines, policy, saving, onChange }: AgentRowProps) {
  const id = (field: Parameters<typeof fieldId>[1]) => fieldId(agent.key, field);
  const errorOf = (field: AgentField) => problems.find((p) => typeof p.field === "object" && p.field.field === field)?.message;
  // What a control with a problem says about itself, beside the danger border.
  const invalid = (field: AgentField) =>
    errorOf(field) ? { "aria-invalid": true, "aria-describedby": errorId(id(field)), className: "border-danger" } : {};
  // A select takes no readOnly, so while saving it takes no change instead.
  const choose = (change: (value: string) => Partial<DraftAgent>) => (event: { target: { value: string } }) =>
    !saving && onChange(change(event.target.value));
  const lines = agent.prompt === "" ? 0 : agent.prompt.split("\n").length;
  const defaultPrompt = agent.coordinates && agent.prompt.trim() === "";
  const shipped = shippedProfiles(policy);
  const profileKnown = agent.profile === "" || policy.profiles.some((p) => p.name === agent.profile);

  return (
    <div className="flex flex-col gap-4 border-t border-divider bg-surface-0 px-4 py-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Name" htmlFor={id("name")} hint="Lowercase letters, digits and dashes." error={errorOf("name")}>
          <Input
            id={id("name")}
            mono
            value={agent.name}
            readOnly={saving}
            onChange={(e) => onChange({ name: e.target.value.toLowerCase() })}
            {...invalid("name")}
          />
        </Field>
        <Field label="Description" htmlFor={id("description")} hint="One line. The coordinator reads it to choose who works next.">
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
        aside={lines === 1 ? "1 line" : lines > 1 ? `${lines} lines` : undefined}
        error={errorOf("prompt")}
        hint={
          defaultPrompt
            ? "Empty: it uses Agentry's coordinator prompt for a team of these agents."
            : "Its standing instructions, at the start of every turn. Tab moves to the next field."
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
            minHeight={180}
            maxHeight={500}
            readOnly={saving}
            placeholder={agent.coordinates ? "Empty: it uses Agentry's coordinator prompt for a team of these agents." : undefined}
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
              ? `${agent.profile} (missing from policy.toml): every call it makes is denied until it's back. Create team offers to add it, or pick another.`
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

      <p className="text-[12px] text-ink-muted">Knowledge and where it runs: on its own page, once the team is created.</p>
    </div>
  );
}
