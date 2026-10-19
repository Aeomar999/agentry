# Networking team template

A team you add from Teams, like Writing, Research, Design and Visionaries, that takes on any networking work: finding why something can't reach something else, and changing the config that decides it. This note is step 1; nothing is built yet.

## What it takes on

Its prompts name all of it:

- **Connectivity:** DNS, routing, ports, MTU, latency and packet loss, "it can't reach X", with ping, dig, ss, ip, traceroute/mtr, curl, and tcpdump where it is allowed.
- **TLS and certificates:** CAs, chains, SANs, expiry, ACME, trust stores, and Agentry's own `-https` CA.
- **Reverse proxies and load balancers:** Traefik, nginx, Caddy, HAProxy: domains, routing rules, headers, websockets.
- **VPNs and overlays:** WireGuard, Tailscale, the Hawser network, Docker and swarm overlay networks.
- **Firewalls and exposure:** ufw, nftables/iptables, cloud security groups: as little exposed as will do, and what is public and why.
- **DNS records and providers**, and **network docs and diagrams** for a setup.

## The team

`{ID: "networking", Team: "Networking", Folder: "networking"}`, Description: *"Diagnoses and changes networks: connectivity, DNS, TLS, proxies, VPNs and firewalls. Works on config in repos, proves each change from where it matters, and touches nothing live without your go-ahead."*

| Name | Role | Profile | Colour | Column | Description |
|---|---|---|---|---|---|
| network-lead | coordinator | read-only | orange | | Takes every networking task, splits and plans it, and decides who works on it next and what needs your go-ahead |
| network-engineer | builds | developer | blue | | Investigates and changes the config in the repo: proxy and firewall config, compose and stack files, IaC, docs and diagrams |
| network-verifier | checks | network-diagnosis (new) | teal | Reviews | Proves a change works, or finds where it breaks, from inside the host or container, the LAN, the private network and the public internet, with the evidence |
| network-security | checks | network-diagnosis (new) | violet | Reviews | Checks every change for exposure, auth, TLS and a tested rollback before it lands |

The usual path is lead › engineer › security › verifier. For "it can't reach X" the lead sends the verifier first, to find where it breaks, then the engineer to fix it. Agents can run on other workers, so the lead picks the verifier's vantage point with `"machine"` on the assign; a public check comes from a worker outside the network, or asks you to run one command and paste what it prints.

Both checkers end with `Verdict: approved` or `Verdict: changes needed`, like the tester and the fact-checker.

## Profiles: what the shipped policy decides today

A throwaway test ran `Decide` on the shipped default.toml with `use_for_everyone` commented out, as the tests do. No rule in read-only, read-mostly or developer mentions a network command, so every one of them falls to `otherwise = "ask"`:

| Command | read-only | read-mostly | developer | autonomous |
|---|---|---|---|---|
| `ping -c 3 host`, `dig`, `ss -tlnp`, `ip route`, `ip addr`, `traceroute`, `mtr --report`, `openssl s_client -connect host:443`, `tcpdump`, `ufw status`, `nft list ruleset`, `docker network ls`, `wg show` | ask (otherwise) | ask (otherwise) | ask (otherwise) | allow (otherwise) |
| `curl -sv https://host` | ask (otherwise) | ask (otherwise) | ask (otherwise) | ask ("acts outside this machine: you approve it") |
| `ufw allow 443`, `ip route add ...`, `systemctl restart nginx` | ask (otherwise) | ask (otherwise) | ask (otherwise) | **allow** (otherwise) |
| `sudo wg show` | ask (otherwise) | ask (otherwise) | deny ("never sudo") | **allow** (otherwise) |

So read-mostly is no better than read-only here: its one extra rule reads the task board. Diagnosis would ask you for every `dig`, and autonomous would change a live firewall, or run anything with sudo, without asking. No shipped profile lets diagnosis run while every live change asks.

Two facts shape the rest:

- The shipped policy sets `use_for_everyone = "allow-everything"`. Until you turn that off, every agent's own profile is ignored and the prompts are the only guard. The README and the lead's prompt should say so.
- Firewall and WireGuard reads (`ufw status`, `nft list ruleset`, `iptables -L`, `wg show`) need root, and sudo asks or is denied in every profile but autonomous. Without sudo they fail harmlessly, so an agent reads the config files in the repo instead, or asks you.

## Recommendation: one new shipped profile, `network-diagnosis`

Diagnosis runs as a short list of fixed forms. Everything else, including every live change, asks you. Profiles have no inheritance, so the profile is read-only's rules copied, with two changes:

- Read-only's first rule, "never touch secrets", also denies the network key files.
- The network rules below go in after read-only's "writes no files from the shell" and before every rule that allows a shell command. If they went first, `ping -c 1 -W 1 host > out.txt` would match the network allow before the redirect deny saw it.

How the patterns match (`internal/policy/glob.go`): `*` crosses spaces, `#` is a number, and a trailing ` *` may match nothing. A form whose arguments are a trailing ` *` gets two option asks, `'<fixed part> -*'` for an option straight after the fixed part and `'<fixed part> * -*'` for one after an argument. The literal `'P -*'` alone would miss `ping -c 3 -W 2 -f host`. A form whose `*` is joined to a word (`show*`, `status*`) needs one, `'<form> -*'`, which covers both places. An exact form (`ss -tlnp`) allows nothing else, so it needs none.

```toml
[profiles.network-diagnosis]
why = "For agents that check network work: they look at how traffic flows with a fixed set of read-only commands, and change nothing. Every live change asks you."
otherwise = "ask"
unreadable = "ask"
scratch = ['/tmp/claude-*/**']

  [[profiles.network-diagnosis.rules]]
  name = "never touch secrets"
  decision = "deny"
  paths = [
    '.env', '.env.*', '~/.ssh/**', '~/.aws/**', '~/.config/gcloud/**', '~/.netrc', '~/.git-credentials', '**/id_rsa', '**/id_ed25519',
    '/etc/wireguard/**', '**/acme.json', '**/privkey*.pem',
  ]
  why = "Keys and passwords never reach a model: WireGuard's keys, and the private keys of certificates (Traefik keeps them in acme.json)."

  # read-only's rules from "keeps its own notes" to "writes no files from the shell", unchanged

  [[profiles.network-diagnosis.rules]]
  name = "never prints a private key"
  decision = "deny"
  runs_any = ['*wg showconf*', '*wg show* dump*', '*wg show * private-key*', '*wg show * preshared-keys*', '*wg genkey*']
  why = "These print a WireGuard private or preshared key, or make one. wg show alone shows the interfaces and their public keys."

  [[profiles.network-diagnosis.rules]]
  name = "changes live network state"
  decision = "ask"
  runs_any = [
    'ufw *allow*', 'ufw *deny*', 'ufw *reject*', 'ufw *limit*', 'ufw *delete*', 'ufw *insert*', 'ufw *prepend*', 'ufw *route*',
    'ufw *default*', 'ufw *enable*', 'ufw *disable*', 'ufw *reset*', 'ufw *reload*', 'ufw *logging*',
    'ip*tables *-A*', 'ip*tables *-I*', 'ip*tables *-D*', 'ip*tables *-R*', 'ip*tables *-F*', 'ip*tables *-X*', 'ip*tables *-N*',
    'ip*tables *-P*', 'ip*tables *-Z*', 'ip*tables *-E*', 'ip*tables *--*', 'ip*tables-restore*',
    'nft *add *', 'nft *insert *', 'nft *create *', 'nft *delete *', 'nft *destroy *', 'nft *flush *', 'nft *replace *',
    'nft *reset *', 'nft *rename *', 'nft -f*', 'nft * -f*',
    'ip * add *', 'ip * del *', 'ip * delete *', 'ip * change *', 'ip * replace *', 'ip * append *', 'ip * prepend *',
    'ip * flush *', 'ip * set *',
    'wg set *', 'wg setconf *', 'wg addconf *', 'wg syncconf *', 'wg-quick *',
    'systemctl *start*', 'systemctl *stop*', 'systemctl *reload*',
  ]
  why = "This changes a live machine's firewall, routes, links, VPN or services, and a mistake there can lock people out. Your approval here is the go-ahead the task needs: the task must already show the exact commands, what they should do, and a rollback that was tested."

  [[profiles.network-diagnosis.rules]]
  name = "network commands take no other options"
  decision = "ask"
  runs_any = [
    'ping -c # -W # -*', 'ping -c # -W # * -*', 'dig +short -*', 'dig +short * -*',
    'ip addr show* -*', 'ip route show* -*', 'ip link show* -*',
    'traceroute -n -*', 'traceroute -n * -*', 'mtr --report -c # -*', 'mtr --report -c # * -*',
    'docker network inspect -*', 'docker network inspect * -*', 'ufw status* -*',
    'nft *;*',
  ]
  why = "The diagnosis forms are allowed only as they are. Another option can read a file (dig -f), send chosen bytes (ping -p) or change how the tool runs, so it asks. nft takes several commands joined with ; in one argument, so that asks too."

  [[profiles.network-diagnosis.rules]]
  name = "diagnoses the network"
  decision = "allow"
  runs_only = [
    'ping -c # -W # *', 'dig +short *', 'ss -tlnp', 'ss -tunap',
    'ip addr show*', 'ip route show*', 'ip link show*',
    'traceroute -n *', 'mtr --report -c # *',
    'docker network ls', 'docker network inspect *',
    # firewall and WireGuard reads need root; without it they fail and change nothing
    'ufw status*', 'nft list ruleset', 'iptables -S', 'iptables -L -n -v', 'wg show',
  ]
  why = "These look at addresses, routes, sockets, firewall rules and paths across the network, and change nothing."

  # read-only's rules from "runs nothing that changes files" on, unchanged
```

The allow has no `where`: its arguments are hosts and addresses, not files.

No form was dropped. Each one with arguments is closed by its option asks, and the arguments left can only be hosts, addresses or `ip`'s own words (`dev eth0`, `table main`). An `ip` change never reaches the allow either: `ip addr add ...` doesn't start with `ip addr show`.

### Decisions

- **curl, openssl, nc and tcpdump are not allowed forms.** They ask through read-only's `otherwise = "ask"`. Safe fixed forms for them are a follow-up.
- **A command wrapped in `timeout` asks.** `timeout` isn't one of the wrappers Agentry looks through (`internal/policy/shell.go`), so `timeout 5 ping ...` matches no allowed form. The key-printer denies start with `*`, so `timeout`, `sudo` or anything else in front can't get past them. The prompts tell agents to use each tool's own limits instead: `ping -c # -W #`, `dig +time= +tries=`, `mtr -c #`.
- **`ss` joins `clusteringTools`** in `internal/policy/shell.go`, with a test, and default.toml's clusters comment lists it. Then a rule naming one of ss's short options matches it inside a cluster (`-tlnpK`), as it already does for sort.
- **Pipes ask.** `ss -tlnp | grep :443` asks: `runs_only` needs every command in the call to match one rule's list, and `grep` is in read-only's list, not this one. Agents run the form alone and read the output. The probe test confirms it.

### Deferred follow-ups

1. A diagnosis profile for network-engineer: developer's rules plus these.
2. The network key paths as secrets in every profile, not only this one.
3. Fixed forms for curl, openssl, nc and tcpdump.

### Who gets what

- **network-lead: read-only**, like every other coordinator. It plans and judges; it runs nothing on the network.
- **network-engineer: developer.** It edits and commits config in the repo, which only developer allows. Its network commands ask, which is acceptable because the verifier does the probing. Developer already denies sudo.
- **network-verifier and network-security: network-diagnosis.** They probe without asking and change nothing: Edit and Write are denied, so their evidence and findings go in their reports, as the reviewer's do.

This is the smallest change that lets diagnosis run without asking while every live change asks: one profile, used by two agents. A team of read-mostly agents would ask you for every `dig`.

## Safety rules for the prompts

Every prompt carries these, and the lead's carries them as its rules for deciding:

- A networking mistake locks people out. The team works on config in repos and proposes live changes. It never changes a live machine's firewall, routes, VPN, DNS records or proxy, or restarts a network service, without your explicit go-ahead in this task.
- Every proposed live change comes with the exact commands, the effect it should have, how to check it worked, and a rollback that was tested, plus a way back in if access drops: a timed revert (`at now + 5 minutes` running the rollback, cancelled once you are back in) or a console you can reach without the network.
- Change one thing at a time, from a session that won't be cut off, and keep the old config beside the new.
- Never print, paste or commit a secret: private keys, tokens, WireGuard private and preshared keys, CA keys, or `acme.json`. Mask them in evidence (`PrivateKey = (hidden)`).
- Say what is public and why. A new open port or a public route needs a reason in the change.

## What the build step touches

- `internal/agents/teams.go`: the Networking template after Visionaries, which adds no feature.
- `internal/agents/defaults/networking/{network-lead,network-engineer,network-verifier,network-security}.md`: about 250 to 400 words each, in the voice of `writing/` and `visionaries/`: "You are the ...", a "How to ..." list, the checkers ending on their verdict line.
- `internal/policy/default.toml`: the `network-diagnosis` profile, copied to a new `shipped/07-<next version>.toml` (TestEveryShippedProfileIsKept). Adding a profile also changes `policy/file_test.go` (TestTheDefaultHasTheFiveProfiles), the profile loops in `policy/default_test.go`, and the profile count in `web/api_test.go` TestThePolicyShowsWithItsProblem. A new `default_test.go` test checks the rules above: each allowed form allows, an option before or after its arguments asks, the live changes ask, and the key printers deny, also behind `sudo` or `timeout`. The same step adds `ss` to `clusteringTools`.
- The missing-profiles offer needs no change: an existing install's policy.toml lacks `network-diagnosis`, `POST /api/teams` answers 409 `{missingProfiles}`, and Add the profiles and the team calls `policy.AddProfile` with the shipped text. The UI reads templates from the API, so Teams shows the new one with no UI change.
- `README.md`, lines 109 and 256: add Networking to the teams you can add.
- Tests:
  - `agents_test.go`: Networking in TestTheTemplatesAndTheirAgents' names and team list, and a TestTheNetworkingTeam like TestTheVisionariesTeam: four agents, network-lead the coordinator, network-security and network-verifier in Reviews, the profiles above.
  - `prompts_policy_test.go`, which already parses the shipped policy: a test that every template's every profile is in the shipped policy, and a TestTheNetworkingTeamDiagnosesAndAsksBeforeChanging that runs the commands the prompts name against each agent's profile. TestEveryTemplateHasACoordinatorAndPrompts and TestTheDefaultPromptsOnlyAskForCommandsThePolicyPermits cover the new prompts as they are.

## Probe results

The profile above, with its placeholders filled from read-only, was appended to the shipped policy and run through `Decide` in a throwaway test: 91 cases, all as expected (16 allow, 69 ask, 6 deny). No rule changed and no form was dropped.

| Probe | Cases | Decision | Deciding rule |
|---|---|---|---|
| Allowed form | 16 | allow | diagnoses the network |
| `--extra` after the fixed part | 16 | ask | network commands take no other options (9); changes live network state (2, `iptables`, by `ip*tables *--*`); otherwise (5, exact forms) |
| `--extra` at the end | 16 | ask | the same split as above |
| Wrapped in `timeout 5` | 16 | ask | otherwise |
| Chained with `&& echo hi` | 16 | ask | otherwise |
| Key printers (`wg show wg0 dump`, also behind `sudo`; `timeout 5 wg showconf wg0`) | 3 | deny | never prints a private key |
| `ss -tK` (clustered options) | 1 | ask | otherwise |
| `nft list ruleset \; list tables` | 1 | ask | network commands take no other options |
| Pipe (`ss -tlnp \| grep :443`) | 1 | ask | otherwise |
| Redirect (`ping ... > out.txt`) | 1 | deny | writes no files from the shell |
| Live changes (`ufw allow 443`, `ip route add ...`, `systemctl restart nginx`) | 3 | ask | changes live network state |
| Secret path (Read `/etc/wireguard/wg0.conf`) | 1 | deny | never touch secrets |

A wrapped, chained or piped form asks because the wrapper or the other command is not on any list. The full output, one line per case with its deciding rule, is in the task's shared files: `.agentry/files/networking-profile-results.md`, beside the full profile in `.agentry/files/network-diagnosis.toml`.
