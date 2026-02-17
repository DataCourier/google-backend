import { useState, useEffect, useRef, useCallback } from "react";
import { updateRecord } from "./api";

const POSITION_KEY = "yt-positions";
const MAX_HISTORY = 50;

function loadPositions() {
  try {
    return JSON.parse(localStorage.getItem(POSITION_KEY)) || {};
  } catch {
    return {};
  }
}

function savePosition(videoId, time) {
  const positions = loadPositions();
  positions[videoId] = { time, ts: Date.now() };
  // Keep only last 50 entries by recency
  const entries = Object.entries(positions);
  if (entries.length > MAX_HISTORY) {
    entries.sort((a, b) => b[1].ts - a[1].ts);
    const trimmed = Object.fromEntries(entries.slice(0, MAX_HISTORY));
    localStorage.setItem(POSITION_KEY, JSON.stringify(trimmed));
  } else {
    localStorage.setItem(POSITION_KEY, JSON.stringify(positions));
  }
}

function getSavedPosition(videoId) {
  return loadPositions()[videoId]?.time || 0;
}

// Load YT IFrame API once globally
let ytApiReady = false;
let ytApiCallbacks = [];
function ensureYTApi() {
  if (ytApiReady) return Promise.resolve();
  if (window.YT?.Player) {
    ytApiReady = true;
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    ytApiCallbacks.push(resolve);
    if (!document.querySelector('script[src*="youtube.com/iframe_api"]')) {
      const tag = document.createElement("script");
      tag.src = "https://www.youtube.com/iframe_api";
      document.head.appendChild(tag);
      window.onYouTubeIframeAPIReady = () => {
        ytApiReady = true;
        ytApiCallbacks.forEach((cb) => cb());
        ytApiCallbacks = [];
      };
    }
  });
}

export default function VideoPage({ video, onBack, onUpdated }) {
  const [notes, setNotes] = useState(video.notes || "");
  const [saveStatus, setSaveStatus] = useState("");
  const timerRef = useRef(null);
  const lastSavedRef = useRef(video.notes || "");
  const versionRef = useRef(video.version || 0);
  const [progress, setProgress] = useState(0);
  const playerRef = useRef(null);
  const positionIntervalRef = useRef(null);
  const containerRef = useRef(null);

  // Reset notes when video changes
  useEffect(() => {
    setNotes(video.notes || "");
    lastSavedRef.current = video.notes || "";
    versionRef.current = video.version || 0;
  }, [video.id]);

  // YouTube player with position tracking
  useEffect(() => {
    let destroyed = false;

    ensureYTApi().then(() => {
      if (destroyed) return;
      const startAt = getSavedPosition(video.video_id);
      playerRef.current = new window.YT.Player(containerRef.current, {
        videoId: video.video_id,
        playerVars: { autoplay: 0 },
        events: {
          onReady: () => {
            if (startAt > 30) {
              playerRef.current.mute();
              playerRef.current.seekTo(startAt, true);
              playerRef.current.playVideo();
              setTimeout(() => {
                if (playerRef.current) {
                  playerRef.current.pauseVideo();
                  playerRef.current.unMute();
                }
              }, 1000);
            }
            // Save position every 10s, update progress every 1s
            positionIntervalRef.current = setInterval(() => {
              const p = playerRef.current;
              if (!p?.getCurrentTime || !p?.getDuration) return;
              const t = p.getCurrentTime();
              const d = p.getDuration();
              if (d > 0) setProgress(t / d);
              if (t > 0 && Math.round(t) % 10 === 0) savePosition(video.video_id, t);
            }, 1000);
          },
        },
      });
    });

    return () => {
      destroyed = true;
      clearInterval(positionIntervalRef.current);
      // Save final position on unmount
      if (playerRef.current?.getCurrentTime) {
        const t = playerRef.current.getCurrentTime();
        if (t > 0) savePosition(video.video_id, t);
      }
      playerRef.current?.destroy?.();
      playerRef.current = null;
    };
  }, [video.video_id]);

  // Auto-save with 2s debounce
  useEffect(() => {
    if (notes === lastSavedRef.current) return;

    setSaveStatus("unsaved");
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(async () => {
      setSaveStatus("saving...");
      try {
        const res = await updateRecord("videos", video.id, {
          notes,
          version: versionRef.current,
        });
        lastSavedRef.current = notes;
        versionRef.current = res.data?.version ?? versionRef.current + 1;
        setSaveStatus("saved");
        onUpdated?.();
        setTimeout(() => setSaveStatus((s) => (s === "saved" ? "" : s)), 2000);
      } catch (err) {
        if (err.status === 409) {
          setSaveStatus("conflict - reload to see latest");
        } else {
          setSaveStatus("failed to save");
        }
      }
    }, 2000);

    return () => clearTimeout(timerRef.current);
  }, [notes, video.id]);

  const watched = !!video.watched;
  const liked = !!video.liked;
  const favorited = !!video.favorited;
  const watchLater = !!video.watch_later;

  async function toggle(field) {
    await updateRecord("videos", video.id, { [field]: !video[field] });
    onUpdated?.();
  }

  const publishDate = video.published
    ? new Date(video.published).toLocaleDateString()
    : "";

  return (
    <div className="max-w-4xl mx-auto">
      <button
        onClick={onBack}
        className="text-sm text-neutral-400 hover:text-neutral-100 mb-4 flex items-center gap-1"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
        </svg>
        Back to feed
      </button>

      <div className="aspect-video w-full mb-1">
        <div ref={containerRef} className="w-full h-full rounded-lg" />
      </div>
      <div className="w-full h-1 bg-neutral-800 rounded-full mb-4 overflow-hidden">
        <div
          className="h-full bg-red-600 transition-all duration-1000 ease-linear"
          style={{ width: `${Math.min(progress * 100, 100)}%` }}
        />
      </div>

      <h1 className="text-lg font-semibold text-neutral-100">{video.title}</h1>
      <p className="text-sm text-neutral-400 mt-1">
        {video.channel} {publishDate && `\u00B7 ${publishDate}`}
      </p>

      <div className="flex gap-2 mt-3">
        <button
          onClick={() => toggle("watched")}
          className={`text-sm px-3 py-1 rounded ${
            watched
              ? "bg-green-900 text-green-300"
              : "bg-neutral-800 text-neutral-400"
          }`}
        >
          {watched ? "Watched" : "Unwatched"}
        </button>
        <button
          onClick={() => toggle("liked")}
          className={`text-sm px-3 py-1 rounded ${
            liked
              ? "bg-red-900 text-red-300"
              : "bg-neutral-800 text-neutral-400"
          }`}
        >
          {liked ? "Liked" : "Like"}
        </button>
        <button
          onClick={() => toggle("favorited")}
          className={`text-sm px-3 py-1 rounded ${
            favorited
              ? "bg-yellow-900 text-yellow-300"
              : "bg-neutral-800 text-neutral-400"
          }`}
        >
          {favorited ? "Favorited" : "Favorite"}
        </button>
        <button
          onClick={() => toggle("watch_later")}
          className={`text-sm px-3 py-1 rounded ${
            watchLater
              ? "bg-blue-900 text-blue-300"
              : "bg-neutral-800 text-neutral-400"
          }`}
        >
          {watchLater ? "In Queue" : "Watch Later"}
        </button>
        <a
          href={`https://www.youtube.com/watch?v=${video.video_id}`}
          target="_blank"
          rel="noopener noreferrer"
          className="text-sm px-3 py-1 rounded bg-neutral-800 text-neutral-400 hover:text-red-400"
        >
          Open on YouTube
        </a>
      </div>

      <div className="mt-4">
        <div className="flex items-center gap-2">
          <label className="text-sm font-medium text-neutral-300">Notes</label>
          {saveStatus && (
            <span className={`text-xs ${
              saveStatus === "saved" ? "text-green-500" :
              saveStatus.startsWith("conflict") ? "text-orange-400" :
              saveStatus === "failed to save" ? "text-red-400" :
              "text-neutral-500"
            }`}>
              {saveStatus}
            </span>
          )}
        </div>
        <textarea
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="Add notes about this video..."
          className="mt-1 w-full h-[48rem] p-3 text-base font-mono bg-neutral-900 border border-neutral-700 text-neutral-100 rounded-lg resize-y focus:outline-none focus:ring-2 focus:ring-red-500 focus:border-transparent placeholder-neutral-500"
        />
      </div>
    </div>
  );
}
