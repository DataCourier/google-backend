import { useState, useEffect } from "react";
import VideoCard from "./VideoCard";

export default function Feed({ videos, selectedChannel, onUpdated, onSelectVideo, scrollToVideoId, onScrolled }) {
  const [filter, setFilter] = useState("all");
  const [hideShorts, setHideShorts] = useState(true);

  let filtered = videos;

  if (selectedChannel === "watch_later") {
    filtered = filtered.filter((v) => v.watch_later);
  } else if (selectedChannel === "liked") {
    filtered = filtered.filter((v) => v.liked);
  } else if (selectedChannel === "notes") {
    filtered = filtered.filter((v) => v.notes);
  } else if (selectedChannel) {
    filtered = filtered.filter((v) => v.channel_id === selectedChannel);
  }

  if (hideShorts) {
    filtered = filtered.filter(
      (v) => !v.title?.match(/#shorts/i)
    );
  }

  if (filter === "unwatched") {
    filtered = filtered.filter((v) => !v.watched);
  } else if (filter === "favorited") {
    filtered = filtered.filter((v) => v.favorited);
  }

  // Sort by published date descending
  filtered = [...filtered].sort(
    (a, b) => new Date(b.published || 0) - new Date(a.published || 0)
  );

  useEffect(() => {
    if (!scrollToVideoId) return;
    const el = document.querySelector(`[data-video-id="${scrollToVideoId}"]`);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      el.classList.add("bg-neutral-800");
      setTimeout(() => el.classList.remove("bg-neutral-800"), 2000);
    }
    onScrolled?.();
  }, [scrollToVideoId]);

  return (
    <div>
      <div className="flex gap-2 mb-4">
        {["all", "unwatched", "favorited"].map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={`text-xs px-3 py-1 rounded-full capitalize ${
              filter === f
                ? "bg-red-600 text-white"
                : "bg-neutral-800 text-neutral-400 hover:bg-neutral-700"
            }`}
          >
            {f}
          </button>
        ))}
        <button
          onClick={() => setHideShorts(!hideShorts)}
          className={`text-xs px-3 py-1 rounded-full ${
            hideShorts
              ? "bg-neutral-800 text-neutral-400 hover:bg-neutral-700"
              : "bg-purple-600 text-white"
          }`}
        >
          {hideShorts ? "Show Shorts" : "Hiding Shorts"}
        </button>
        <span className="text-xs text-neutral-400 ml-auto self-center">
          {filtered.length} videos
        </span>
      </div>
      <div className="space-y-2">
        {filtered.map((v) => (
          <VideoCard key={v.id} video={v} onUpdated={onUpdated} onSelect={onSelectVideo} />
        ))}
        {filtered.length === 0 && (
          <p className="text-neutral-500 text-sm text-center py-8">No videos</p>
        )}
      </div>
    </div>
  );
}
