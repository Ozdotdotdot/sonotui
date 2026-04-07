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

/// Returns true if a dotted-decimal IPv4 string falls in Tailscale's CGNAT
/// range (100.64.0.0/10).
fn is_tailscale_str(ip: &str) -> bool {
    let mut parts = ip.split('.');
    let a: u8 = parts.next().and_then(|s| s.parse().ok()).unwrap_or(0);
    let b: u8 = parts.next().and_then(|s| s.parse().ok()).unwrap_or(0);
    a == 100 && (64..=127).contains(&b)
}

/// Browse for `_sonogui._tcp` services for up to `timeout`.
///
/// Deduplicates by mDNS fullname: the same daemon advertising on multiple
/// interfaces (LAN + Tailscale) collapses to one entry, preferring the LAN
/// address over the Tailscale 100.x.x.x address.
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

                // Collect addresses as strings; pick best (LAN IPv4 first).
                let addresses: Vec<String> = info
                    .get_addresses()
                    .iter()
                    .filter(|a| a.is_ipv4())
                    .map(|a| a.to_string())
                    .collect();

                let best = addresses
                    .iter()
                    .find(|h| !is_tailscale_str(h))
                    .or_else(|| addresses.first())
                    .cloned();

                let Some(best_host) = best else { continue };

                match found.get_mut(&fullname) {
                    None => {
                        found.insert(fullname, DaemonInfo { name, host: best_host, port });
                    }
                    Some(existing) => {
                        // Same service seen again (e.g. via a different interface).
                        // Upgrade from Tailscale to LAN if possible.
                        if is_tailscale_str(&existing.host) && !is_tailscale_str(&best_host) {
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
