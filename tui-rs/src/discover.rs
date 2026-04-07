use std::time::{Duration, Instant};

use mdns_sd::{ServiceDaemon, ServiceEvent};

#[derive(Debug, Clone)]
pub struct DaemonInfo {
    pub name: String,
    pub host: String,
    pub port: u16,
}

/// Browse for `_sonogui._tcp` services for up to `timeout`.
/// Returns all resolved instances. Errors are silently swallowed — discovery
/// is best-effort and the caller falls back gracefully.
pub fn discover(timeout: Duration) -> Vec<DaemonInfo> {
    let Ok(mdns) = ServiceDaemon::new() else {
        return vec![];
    };
    let Ok(recv) = mdns.browse("_sonogui._tcp.local.") else {
        let _ = mdns.shutdown();
        return vec![];
    };

    let deadline = Instant::now() + timeout;
    let mut found = vec![];

    while Instant::now() < deadline {
        let remaining = deadline.saturating_duration_since(Instant::now());
        let poll = remaining.min(Duration::from_millis(50));
        match recv.recv_timeout(poll) {
            Ok(ServiceEvent::ServiceResolved(info)) => {
                // Prefer IPv4; fall back to any address.
                let addr = info
                    .get_addresses()
                    .iter()
                    .find(|a| a.is_ipv4())
                    .or_else(|| info.get_addresses().iter().next())
                    .map(|a| a.to_string());

                if let Some(host) = addr {
                    // Use the human-readable instance name (e.g. "oz-server"), not the
                    // full DNS name which includes the service type suffix.
                    let name = info
                        .get_fullname()
                        .split('.')
                        .next()
                        .unwrap_or(info.get_fullname())
                        .to_string();

                    found.push(DaemonInfo {
                        name,
                        host,
                        port: info.get_port(),
                    });
                }
            }
            Ok(_) => {} // SearchStarted, SearchStopped, etc. — ignore
            Err(_) => {} // timeout poll — loop continues
        }
    }

    let _ = mdns.shutdown();
    found
}
