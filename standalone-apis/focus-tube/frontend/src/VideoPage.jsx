import { useState, useEffect, useRef } from "react";
import { updateRecord } from "./api";

export default function VideoPage({ video, onBack, onUpdated }) {
  const [notes, setNotes] = useState(video.notes || "");
  const [saveStatus, setSaveStatus] = useState("");
  const timerRef = useRef(null);
  const lastSavedRef = useRef(video.notes || "");
  const versionRef = useRef(video.version || 0);

  // Reset notes when video changes
  useEffect(() => {
    setNotes(video.notes || "");
    lastSavedRef.current = video.notes || "";
    versionRef.current = video.version || 0;
  }, [video.id]);

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

      <div className="aspect-video w-full mb-4">
        <iframe
          src={`https://www.youtube.com/embed/${video.video_id}`}
          title={video.title}
          className="w-full h-full rounded-lg"
          allowFullScreen
          allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
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
