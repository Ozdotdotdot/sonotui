use std::{
    collections::HashMap,
    time::{Duration, Instant},
};

use mdns_sd::{ServiceDaemon, ServiceEvent};

#[derive(Debug, Clone)]
pub struct DaemonInfo {
    pub name: String,
    pub host: String,
    pub port: u16,
}

/// Returns true if a dotted-decimal IPv4 string falls in the CGNAT range
/// (100.64.0.0/10) used by mesh VPNs: Tailscale, Netbird, Headscale, etc.
/// These addresses are stable and routable from anywhere the VPN is running,
/// making them preferable to LAN IPs which are only reachable locally.
pub fn is_mesh_vpn(ip: &str) -> bool {
    let mut parts = ip.split('.');
    let a: u8 = parts.next().and_then(|s| s.parse().ok()).unwrap_or(0);
    let b: u8 = parts.next().and_then(|s| s.parse().ok()).unwrap_or(0);
    a == 100 && (64..=127).contains(&b)
}

/// Browse for `_sonogui._tcp` services for up to `timeout`.
///
/// Deduplicates by mDNS fullname: the same daemon advertising on multiple
/// interfaces (LAN + mesh VPN) collapses to one entry. Mesh VPN addresses
/// (100.64.0.0/10) are preferred over plain LAN addresses because they are
/// stable and reachable from anywhere the VPN is running, not just locally.
/// Users without a mesh VPN see only their LAN address and are unaffected.
pub fn discover(timeout: Duration) -> Vec<DaemonInfo> {
    let Ok(mdns) = ServiceDaemon::new() else {
        return vec![];
    };
    let Ok(recv) = mdns.browse("_sonogui._tcp.local.") else {
        let _ = mdns.shutdown();
        return vec![];
    };

    let deadline = Instant::now() + timeout;
    // Key: mDNS fullname (unique on the LAN — safe dedup key).
    let mut found: HashMap<String, DaemonInfo> = HashMap::new();

    while Instant::now() < deadline {
        let remaining = deadline.saturating_duration_since(Instant::now());
        let poll = remaining.min(Duration::from_millis(50));
        match recv.recv_timeout(poll) {
            Ok(ServiceEvent::ServiceResolved(info)) => {
                let fullname = info.get_fullname().to_string();
                let port = info.get_port();
                let name = fullname
                    .split('.')
                    .next()
                    .unwrap_or(&fullname)
                    .to_string();

                // Collect IPv4 addresses as strings.
                let addresses: Vec<String> = info
                    .get_addresses()
                    .iter()
                    .filter(|a| a.is_ipv4())
                    .map(|a| a.to_string())
                    .collect();

                // Prefer mesh VPN (CGNAT) address; fall back to any IPv4.
                let best = addresses
                    .iter()
                    .find(|h| is_mesh_vpn(h))
                    .or_else(|| addresses.first())
                    .cloned();

                let Some(best_host) = best else { continue };

                match found.get_mut(&fullname) {
                    None => {
                        found.insert(fullname, DaemonInfo { name, host: best_host, port });
                    }
                    Some(existing) => {
                        // Same service seen again via a different interface.
                        // Upgrade from LAN to mesh VPN if we now have one.
                        if !is_mesh_vpn(&existing.host) && is_mesh_vpn(&best_host) {
                            existing.host = best_host;
                        }
                    }
                }
            }
            Ok(_) => {} // SearchStarted, SearchStopped, etc. — ignore
            Err(_) => {} // timeout poll — loop continues
        }
    }

    let _ = mdns.shutdown();
    found.into_values().collect()
}
