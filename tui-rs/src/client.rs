use std::sync::mpsc::Sender;

use anyhow::{Context, Result};
use futures::StreamExt;
use reqwest::Client;
use reqwest_eventsource::{Event, EventSource};
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

#[derive(Clone)]
pub struct DaemonClient {
    base: String,
    http: Client,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct StatusResponse {
    #[serde(default)]
    pub transport: String,
    #[serde(default)]
    pub track: TrackInfo,
    #[serde(default)]
    pub volume: i32,
    #[serde(default)]
    pub elapsed: i32,
    #[serde(default)]
    pub duration: i32,
    #[serde(default, rename = "is_line_in")]
    pub is_line_in: bool,
    #[serde(default)]
    pub speaker: Option<SpeakerInfo>,
    #[serde(default, rename = "library_ready")]
    pub library_ready: bool,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct TrackInfo {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub artist: String,
    #[serde(default)]
    pub album: String,
    #[serde(default)]
    pub art_url: String,
    #[serde(default)]
    pub duration: i32,
    #[serde(default)]
    pub uri: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct SpeakerInfo {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub uuid: String,
    #[serde(default)]
    pub ip: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct QueueItem {
    #[serde(default)]
    pub position: i32,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub artist: String,
    #[serde(default)]
    pub album: String,
    #[serde(default)]
    pub duration: i32,
    #[serde(default)]
    pub uri: String,
    #[serde(default)]
    pub is_local: bool,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct LibraryEntry {
    #[serde(default)]
    pub name: String,
    #[serde(default, rename = "type")]
    pub entry_type: String,
    #[serde(default)]
    pub path: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub artist: String,
    #[serde(default)]
    pub album: String,
    #[serde(default)]
    pub duration: i32,
    #[serde(default)]
    pub art_hash: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Album {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub artist: String,
    #[serde(default)]
    pub year: i32,
    #[serde(default)]
    pub track_count: i32,
    #[serde(default)]
    pub art_hash: String,
    #[serde(default)]
    pub path: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct AlbumDetail {
    #[serde(flatten)]
    pub album: Album,
    #[serde(default)]
    pub tracks: Vec<LibraryEntry>,
}

#[derive(Debug, Deserialize)]
struct LibraryResponse {
    #[serde(default)]
    entries: Vec<LibraryEntry>,
}

#[derive(Debug, Serialize)]
struct VolumeBody {
    value: i32,
}

#[derive(Debug, Serialize)]
struct VolumeDeltaBody {
    delta: i32,
}

#[derive(Debug, Serialize)]
struct SpeakerBody {
    uuid: String,
}

#[derive(Debug, Serialize)]
struct SeekBody {
    seconds: i32,
}

#[derive(Debug, Serialize)]
struct ReorderBody {
    from: i32,
    to: i32,
}

#[derive(Debug, Serialize)]
struct BatchBody {
    paths: Vec<String>,
}

#[derive(Debug, Clone)]
pub struct DaemonEvent {
    pub kind: String,
    pub payload: Map<String, Value>,
}

impl DaemonClient {
    pub fn new(host: &str, port: u16) -> Self {
        Self {
            base: format!("http://{host}:{port}"),
            http: Client::new(),
        }
    }

    pub async fn status(&self) -> Result<StatusResponse> {
        self.http
            .get(format!("{}/status", self.base))
            .send()
            .await
            .context("GET /status")?
            .error_for_status()
            .context("/status returned error")?
            .json()
            .await
            .context("parse /status")
    }

    pub async fn queue(&self) -> Result<Vec<QueueItem>> {
        self.http
            .get(format!("{}/queue", self.base))
            .send()
            .await
            .context("GET /queue")?
            .error_for_status()
            .context("/queue returned error")?
            .json()
            .await
            .context("parse /queue")
    }

    pub async fn speakers(&self) -> Result<Vec<SpeakerInfo>> {
        self.http
            .get(format!("{}/speakers", self.base))
            .send()
            .await
            .context("GET /speakers")?
            .error_for_status()
            .context("/speakers returned error")?
            .json()
            .await
            .context("parse /speakers")
    }

    pub async fn library(&self, path: &str) -> Result<Vec<LibraryEntry>> {
        let clean = path.trim_start_matches('/');
        let url = if clean.is_empty() {
            format!("{}/library", self.base)
        } else {
            format!("{}/library/{}", self.base, clean)
        };
        let resp = self
            .http
            .get(url)
            .send()
            .await
            .context("GET /library")?
            .error_for_status()
            .context("/library returned error")?;
        Ok(resp
            .json::<LibraryResponse>()
            .await
            .context("parse /library")?
            .entries)
    }

    pub async fn library_search(&self, query: &str) -> Result<Vec<LibraryEntry>> {
        self.http
            .get(format!("{}/library/search", self.base))
            .query(&[("q", query)])
            .send()
            .await
            .context("GET /library/search")?
            .error_for_status()
            .context("/library/search returned error")?
            .json()
            .await
            .context("parse /library/search")
    }

    pub async fn albums(&self) -> Result<Vec<Album>> {
        self.http
            .get(format!("{}/albums", self.base))
            .send()
            .await
            .context("GET /albums")?
            .error_for_status()
            .context("/albums returned error")?
            .json()
            .await
            .context("parse /albums")
    }

    pub async fn albums_search(&self, query: &str) -> Result<Vec<Album>> {
        self.http
            .get(format!("{}/albums/search", self.base))
            .query(&[("q", query)])
            .send()
            .await
            .context("GET /albums/search")?
            .error_for_status()
            .context("/albums/search returned error")?
            .json()
            .await
            .context("parse /albums/search")
    }

    pub async fn album_detail(&self, id: &str) -> Result<AlbumDetail> {
        self.http
            .get(format!("{}/albums/{id}", self.base))
            .send()
            .await
            .context("GET /albums/{id}")?
            .error_for_status()
            .context("/albums/{id} returned error")?
            .json()
            .await
            .context("parse /albums/{id}")
    }

    pub async fn rescan(&self) -> Result<()> {
        self.post_empty("/library/rescan").await
    }

    pub async fn reconnect(&self) -> Result<()> {
        self.post_empty("/reconnect").await
    }

    pub async fn seek(&self, seconds: i32) -> Result<()> {
        self.post_json("/seek", &SeekBody { seconds }).await
    }

    pub async fn play(&self) -> Result<()> {
        self.post_empty("/play").await
    }

    pub async fn pause(&self) -> Result<()> {
        self.post_empty("/pause").await
    }

    pub async fn stop(&self) -> Result<()> {
        self.post_empty("/stop").await
    }

    pub async fn next(&self) -> Result<()> {
        self.post_empty("/next").await
    }

    pub async fn prev(&self) -> Result<()> {
        self.post_empty("/prev").await
    }

    pub async fn linein(&self) -> Result<()> {
        self.post_empty("/linein").await
    }

    pub async fn volume_set(&self, value: i32) -> Result<()> {
        self.post_json("/volume", &VolumeBody { value }).await
    }

    pub async fn volume_relative(&self, delta: i32) -> Result<()> {
        self.post_json("/volume/relative", &VolumeDeltaBody { delta })
            .await
    }

    pub async fn set_active_speaker(&self, uuid: &str) -> Result<()> {
        self.post_json(
            "/speakers/active",
            &SpeakerBody {
                uuid: uuid.to_string(),
            },
        )
        .await
    }

    pub async fn queue_play(&self, pos: i32) -> Result<()> {
        self.post_empty(&format!("/queue/{pos}/play")).await
    }

    pub async fn queue_delete(&self, pos: i32) -> Result<()> {
        self.http
            .delete(format!("{}/queue/{pos}", self.base))
            .send()
            .await
            .context("DELETE /queue/{pos}")?
            .error_for_status()
            .context("/queue/{pos} returned error")?;
        Ok(())
    }

    pub async fn queue_clear(&self) -> Result<()> {
        self.http
            .delete(format!("{}/queue", self.base))
            .send()
            .await
            .context("DELETE /queue")?
            .error_for_status()
            .context("/queue returned error")?;
        Ok(())
    }

    pub async fn queue_reorder(&self, from: i32, to: i32) -> Result<()> {
        self.post_json("/queue/reorder", &ReorderBody { from, to })
            .await
    }

    pub async fn queue_batch(&self, paths: Vec<String>) -> Result<()> {
        self.post_json("/queue/batch", &BatchBody { paths }).await
    }

    pub async fn fetch_art_bytes(&self, url: &str) -> Result<Vec<u8>> {
        let resp = self
            .http
            .get(url)
            .timeout(std::time::Duration::from_secs(5))
            .send()
            .await
            .context("fetch art")?
            .error_for_status()
            .context("art request returned error")?;
        Ok(resp.bytes().await.context("read art bytes")?.to_vec())
    }

    pub async fn stream_events(&self, tx: Sender<DaemonEvent>) -> Result<()> {
        let request = self.http.get(format!("{}/events", self.base));
        let mut source = EventSource::new(request).context("connect /events")?;

        while let Some(event) = source.next().await {
            match event.context("SSE stream error")? {
                Event::Open => {}
                Event::Message(msg) => {
                    let value: Value =
                        serde_json::from_str(&msg.data).context("parse SSE payload")?;
                    let Some(payload) = value.as_object().cloned() else {
                        continue;
                    };
                    let Some(kind) = payload.get("type").and_then(|value| value.as_str()) else {
                        continue;
                    };
                    if tx
                        .send(DaemonEvent {
                            kind: kind.to_string(),
                            payload,
                        })
                        .is_err()
                    {
                        break;
                    }
                }
            }
        }

        source.close();
        Ok(())
    }

    async fn post_empty(&self, path: &str) -> Result<()> {
        self.http
            .post(format!("{}{}", self.base, path))
            .send()
            .await
            .with_context(|| format!("POST {path}"))?
            .error_for_status()
            .with_context(|| format!("{path} returned error"))?;
        Ok(())
    }

    async fn post_json<T: Serialize + ?Sized>(&self, path: &str, body: &T) -> Result<()> {
        self.http
            .post(format!("{}{}", self.base, path))
            .json(body)
            .send()
            .await
            .with_context(|| format!("POST {path}"))?
            .error_for_status()
            .with_context(|| format!("{path} returned error"))?;
        Ok(())
    }
}
