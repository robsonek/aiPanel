# ADR-003: nftables as the Sole Firewall Management Layer

- **Status:** Accepted
- **Date:** 2026-02-06

## Context

aiPanel must configure and manage a host-level firewall as part of its security-by-default baseline (NFR-SEC-001). The firewall must be active immediately after installation, must support dynamic rule management by the panel (e.g., opening ports for new sites, blocking IPs via fail2ban integration), and must be reliable under automated management without human intervention.

The target platform is Debian 13 (Trixie), which ships with nftables as the default packet filtering framework. iptables is available only as a compatibility layer (`iptables-nft`) that translates iptables commands into nftables rules. ufw (Uncomplicated Firewall) is not installed by default on Debian 13 and internally wraps iptables, which in turn wraps nftables — creating a three-layer abstraction chain.

The panel needs to programmatically manage firewall rules with the following requirements:
1. **Atomic rule application** — partial rule updates must not leave the firewall in an inconsistent state.
2. **Template-driven configuration** — rules are generated from Go `text/template` templates based on panel state (sites, services, fail2ban bans).
3. **Auditability** — every firewall change must be traceable in the audit log.
4. **Predictability** — the panel must be the sole owner of firewall rules; no conflicts with other management tools.

## Decision

### nftables as the only firewall layer

aiPanel uses nftables directly as the sole firewall management layer. No abstraction wrappers (ufw, firewalld, iptables-nft) are installed or supported.

### Panel-managed ruleset architecture

The panel manages the nftables ruleset through the following architecture:

1. **Template-based generation:** The nftables ruleset is generated from a Go `text/template` file (`configs/templates/nftables.conf.tmpl`). The template receives the current panel state (enabled services, site ports, banned IPs, custom rules) and produces a complete nftables configuration file.

2. **Atomic ruleset replacement:** Instead of incrementally adding or removing individual rules (which risks partial application failures), the panel uses nftables' atomic ruleset replacement:
   ```
   nft -f /etc/nftables.conf
   ```
   This command replaces the entire active ruleset in a single kernel transaction. Either all rules are applied, or none are — there is no intermediate inconsistent state.

3. **Full ruleset regeneration on every change:** When the panel needs to modify the firewall (new site, blocked IP, service change), it:
   1. Reads current panel state from the database.
   2. Renders the complete nftables configuration from the template.
   3. Validates the generated configuration with `nft -c -f /etc/nftables.conf` (dry-run check).
   4. Applies the configuration atomically with `nft -f /etc/nftables.conf`.
   5. Records the change in the audit log.

4. **Drift detection:** The panel periodically compares the active nftables ruleset (`nft list ruleset`) with the expected ruleset generated from the template. Any drift (manual edits, external tools) triggers an alert and optional auto-remediation.

### Baseline ruleset structure

The default nftables ruleset installed by the panel follows this structure:

```
table inet filter {
    chain input {
        type filter hook input priority filter; policy drop;

        # Connection tracking
        ct state established,related accept
        ct state invalid drop

        # Loopback
        iif "lo" accept

        # ICMP (rate-limited)
        ip protocol icmp limit rate 10/second accept
        ip6 nexthdr icmpv6 limit rate 10/second accept

        # SSH (panel-managed port, rate-limited)
        tcp dport {ssh_port} ct state new limit rate 5/minute accept

        # Panel HTTPS
        tcp dport {panel_port} accept

        # Site ports (HTTP/HTTPS)
        tcp dport { 80, 443 } accept

        # fail2ban banned IPs (dynamic set)
        ip saddr @banned_v4 drop
        ip6 saddr @banned_v6 drop
    }

    chain forward {
        type filter hook forward priority filter; policy drop;
    }

    chain output {
        type filter hook output priority filter; policy accept;
    }

    # Dynamic sets for fail2ban integration
    set banned_v4 {
        type ipv4_addr
        flags timeout
    }

    set banned_v6 {
        type ipv6_addr
        flags timeout
    }
}
```

### fail2ban integration

fail2ban integrates with nftables through nftables sets rather than individual rule insertion:
- Banned IPv4 addresses are added to the `banned_v4` set with a timeout.
- Banned IPv6 addresses are added to the `banned_v6` set with a timeout.
- This avoids fail2ban inserting and removing individual rules, which would conflict with the panel's atomic ruleset management.

### System adapter

The nftables adapter is implemented in `internal/platform/systemd/adapter_nftables.go` and exposes the interface defined in `pkg/adapter/nftables.go`:

```go
type NftablesAdapter interface {
    Apply(ctx context.Context, config NftablesConfig) error
    Validate(ctx context.Context, configPath string) error
    ListRuleset(ctx context.Context) (string, error)
    AddToBannedSet(ctx context.Context, ip string, timeout time.Duration) error
    RemoveFromBannedSet(ctx context.Context, ip string) error
}
```

## Consequences

### Positive

- **Atomic application** — nftables' native `nft -f` command replaces the entire ruleset in a single kernel transaction, eliminating the risk of partial rule application that plagues incremental `iptables` rule management.
- **No abstraction leakage** — by using nftables directly, the panel avoids the complexity of managing rules through wrappers (ufw -> iptables -> nftables) where each layer may introduce translation bugs or incompatibilities.
- **Debian 13 native** — nftables is the default and recommended packet filter for Debian 13. No additional packages need to be installed. The panel works with the OS as designed.
- **Template-driven predictability** — the entire ruleset is generated from a single template with panel state as input. This makes the firewall configuration fully reproducible and auditable.
- **Drift detection** — comparing `nft list ruleset` output with the expected template output enables detection of manual changes or external tool interference.
- **Efficient fail2ban integration** — nftables sets provide O(1) lookup for banned IPs, which is more efficient than the linear rule chain scanning in legacy iptables configurations.
- **Single owner** — the panel is the sole manager of firewall rules, eliminating conflicts with ufw, firewalld, or manual iptables commands.

### Negative

- **Higher barrier to manual intervention** — administrators accustomed to ufw's simple syntax (`ufw allow 80`) will need to use `nft` commands or the panel UI to manage rules. Mitigation: the panel UI provides a user-friendly firewall management interface; documentation covers common nft commands for emergency situations.
- **Full regeneration cost** — every firewall change regenerates and applies the entire ruleset. For very large rulesets (hundreds of custom rules), this could introduce latency. Mitigation: typical hosting panel rulesets are small (< 100 rules); the atomic replacement completes in milliseconds.
- **Template complexity** — the nftables template must handle all combinations of services, ports, banned IPs, and custom rules. Mitigation: the template is well-structured with clear sections and thoroughly tested with fixture-based unit tests.
- **No ufw compatibility** — users who install ufw alongside the panel will create conflicts. Mitigation: the installer checks for and warns about ufw presence; the panel documentation explicitly states that ufw is not supported.

## Alternatives Considered

### ufw (Uncomplicated Firewall)

ufw provides a simple CLI for firewall management and is popular in the Ubuntu ecosystem. However:

1. **ufw is not installed by default on Debian 13.** Adding it introduces an unnecessary dependency.
2. **ufw wraps iptables, which wraps nftables on Debian 13.** This creates a three-layer abstraction: `ufw -> iptables-nft -> nftables kernel`. Each layer introduces potential translation issues and makes debugging harder.
3. **ufw does not support atomic ruleset replacement.** Rules are added and removed individually, creating windows of inconsistent state.
4. **ufw's rule management model conflicts with template-driven generation.** ufw maintains its own state files (`/etc/ufw/user.rules`, `/etc/ufw/user6.rules`) which would need to be kept in sync with the panel's desired state — a fragile approach prone to drift.
5. **ufw lacks native set support** for efficient IP banning. fail2ban would need to insert individual rules rather than adding IPs to a set.

### firewalld

firewalld provides zone-based firewall management and D-Bus API. However:
1. Not installed by default on Debian 13.
2. Adds a daemon dependency (firewalld must be running).
3. Its zone abstraction model does not map cleanly to the panel's per-service, per-site rule requirements.
4. Introduces D-Bus as a communication dependency for firewall management.

### Direct iptables (via iptables-nft)

Using the iptables compatibility layer would work on Debian 13 but provides no advantage over using nftables directly, while inheriting all iptables limitations (no atomic replacement, no sets, less expressive syntax). Since Debian 13 routes iptables commands through nftables anyway, using nftables directly is simpler and more honest.
