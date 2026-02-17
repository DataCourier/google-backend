import { useState } from "react";
import { updateRecord } from "./api";

export default function VideoCard({ video, onUpdated, onSelect }) {
  const [busy, setBusy] = useState(null);
  const watched = !!video.watched;
  const liked = !!video.liked;
  const favorited = !!video.favorited;
  const watchLater = !!video.watch_later;

  async function toggle(field) {
    setBusy(field);
    try {
      await updateRecord("videos", video.id, { [field]: !video[field] });
      onUpdated?.();
    } finally {
      setBusy(null);
    }
  }

  const publishDate = video.published
    ? new Date(video.published).toLocaleDateString()
    : "";

  return (
    <div data-video-id={video.video_id} className="flex gap-3 p-3 rounded-lg border border-neutral-800 hover:border-neutral-600 transition-colors duration-700">
      <div
        className="shrink-0 cursor-pointer"
        onClick={() => onSelect?.(video)}
      >
        <img
          src={video.thumbnail || `https://i.ytimg.com/vi/${video.video_id}/mqdefault.jpg`}
          alt=""
          className="w-40 h-[90px] object-cover rounded"
        />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-start gap-1">
          <span
            onClick={() => onSelect?.(video)}
            className="text-sm font-medium text-neutral-100 hover:text-red-400 line-clamp-2 cursor-pointer flex-1"
          >
            {video.title}
          </span>
          <a
            href={`https://www.youtube.com/watch?v=${video.video_id}`}
            target="_blank"
            rel="noopener noreferrer"
            className="shrink-0 text-neutral-500 hover:text-red-400"
            title="Open on YouTube"
            onClick={(e) => e.stopPropagation()}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
          </a>
        </div>
        <p className="text-xs text-neutral-400 mt-1">
          {video.channel} {publishDate && `\u00B7 ${publishDate}`}
        </p>
        <div className="flex gap-2 mt-2">
          <button
            onClick={() => toggle("watched")}
            disabled={!!busy}
            className={`text-xs px-2 py-0.5 rounded transition-opacity ${busy === "watched" ? "opacity-50" : ""} ${
              watched
                ? "bg-green-900 text-green-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {busy === "watched" ? "..." : watched ? "Watched" : "Unwatched"}
          </button>
          <button
            onClick={() => toggle("liked")}
            disabled={!!busy}
            className={`text-xs px-2 py-0.5 rounded transition-opacity ${busy === "liked" ? "opacity-50" : ""} ${
              liked
                ? "bg-red-900 text-red-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {busy === "liked" ? "..." : liked ? "Liked" : "Like"}
          </button>
          <button
            onClick={() => toggle("favorited")}
            disabled={!!busy}
            className={`text-xs px-2 py-0.5 rounded transition-opacity ${busy === "favorited" ? "opacity-50" : ""} ${
              favorited
                ? "bg-yellow-900 text-yellow-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {busy === "favorited" ? "..." : favorited ? "Favorited" : "Favorite"}
          </button>
          <button
            onClick={() => toggle("watch_later")}
            disabled={!!busy}
            className={`text-xs px-2 py-0.5 rounded transition-opacity ${busy === "watch_later" ? "opacity-50" : ""} ${
              watchLater
                ? "bg-blue-900 text-blue-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {busy === "watch_later" ? "..." : watchLater ? "In Queue" : "Watch Later"}
          </button>
        </div>
        {video.notes && (
          <p className="text-xs text-purple-300/70 mt-1 line-clamp-2 italic">
            {video.notes}
          </p>
        )}
      </div>
    </div>
  );
}
