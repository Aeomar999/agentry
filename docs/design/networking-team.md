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

Profiles have no inheritance, so it is read-only's rules copied, plus:

1. **More secrets denied**, ahead of everything: `/etc/wireguard/**`, `/etc/ssl/private/**`, `**/privkey*.pem`, `**/*.key`, `**/acme.json` (Traefik keeps its certificates' keys there), and Tailscale's and Hawser's state. Agentry's CA folder is protected already.
2. **Ask: changes live network state**, a named rule so the page says why: `ufw` other than `status`, `nft` other than `list`, `iptables`/`ip6tables` other than `-L`/`-S`, `ip ... add|del|change|replace|flush|set`, `wg set`, `wg-quick`, `tailscale up|down|set`, `systemctl start|stop|restart|reload`, `docker network create|rm|connect|disconnect`, `docker service update`, `docker stack deploy`, `nginx -s`, `caddy reload`, `resolvectl dns|domain|flush-caches`. It asks rather than denies, because your approval on the page is the go-ahead the task needs; a deny would make a change you want impossible without editing the policy.
3. **Deny: prints a private key:** `wg showconf *`, `wg show * private-key`, `wg show * preshared-keys`, `wg genkey`.
4. **Allow: diagnosis**, as exact forms, each with an earlier ask rule for any option that writes, sends or listens (the `sed -n * -*` way, since `*` crosses spaces):
   - `ping -c # *`, `dig *`, `host *`, `nslookup ?*`, `getent hosts *`, `resolvectl status|query *`
   - `ip route|addr|link|neigh|rule` with only `show`/`get`, `ss *` (asks on `-K`/`--kill`, `-D`)
   - `traceroute *`, `tracepath *`, `mtr --report *`, `nc -zv * #` (asks on `-l`, `-e`, `-c`)
   - `curl` only in forms that throw the body away (`-sSI`, `-sSv -o /dev/null`, with `--resolve`, `--connect-to`, `--cacert`); asks on anything that writes, posts, uploads or reads a config, and on `file:` URLs. Page bodies can carry instructions aimed at the agent, which is why WebFetch asks.
   - `openssl s_client *`, `openssl x509 -noout *`, `openssl verify *` (asks on `-out`, `-sess_out`, `-keylogfile`, `-msgfile`)
   - `docker network ls|inspect *`, `docker port *`, `tailscale status|ping|netcheck|ip *`, `hawser status *`, `ufw status *`, `nft list *`, `iptables -L|-S *`, `wg show`, `wg show <iface>`
   - `tcpdump` stays ask: it needs root, `-w` writes, `-z` runs a command, and a capture can hold tokens in clear.

Who gets what:

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
- `internal/policy/default.toml`: the `network-diagnosis` profile, copied to a new `shipped/07-<next version>.toml` (TestEveryShippedProfileIsKept). Adding a profile also changes `policy/file_test.go` (TestTheDefaultHasTheFiveProfiles), the profile loops in `policy/default_test.go`, and the profile count in `web/api_test.go` TestThePolicyShowsWithItsProblem. A new `default_test.go` test checks every command above: the diagnosis forms allow, the live changes ask, the key printers deny, and options appended to an allowed form ask.
- The missing-profiles offer needs no change: an existing install's policy.toml lacks `network-diagnosis`, `POST /api/teams` answers 409 `{missingProfiles}`, and Add the profiles and the team calls `policy.AddProfile` with the shipped text. The UI reads templates from the API, so Teams shows the new one with no UI change.
- `README.md`, lines 109 and 256: add Networking to the teams you can add.
- Tests:
  - `agents_test.go`: Networking in TestTheTemplatesAndTheirAgents' names and team list, and a TestTheNetworkingTeam like TestTheVisionariesTeam: four agents, network-lead the coordinator, network-security and network-verifier in Reviews, the profiles above.
  - `prompts_policy_test.go`, which already parses the shipped policy: a test that every template's every profile is in the shipped policy, and a TestTheNetworkingTeamDiagnosesAndAsksBeforeChanging that runs the commands the prompts name against each agent's profile. TestEveryTemplateHasACoordinatorAndPrompts and TestTheDefaultPromptsOnlyAskForCommandsThePolicyPermits cover the new prompts as they are.

## Open questions

- Should the engineer get diagnosis too? That means a second new profile, developer plus the diagnosis rules: another full copy of the developer rules to keep in step. Proposed: not now.
- The network key paths (`/etc/wireguard/**`, `acme.json`, `privkey*.pem`) are secrets in every profile, not only this one. Adding them to the other profiles' "never touch secrets" changes profiles you already have; is that worth its own task?
