import { updateRecord } from "./api";

export default function VideoCard({ video, onUpdated, onSelect }) {
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
    <div className="flex gap-3 p-3 rounded-lg border border-neutral-800 hover:border-neutral-600">
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
            className={`text-xs px-2 py-0.5 rounded ${
              watched
                ? "bg-green-900 text-green-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {watched ? "Watched" : "Unwatched"}
          </button>
          <button
            onClick={() => toggle("liked")}
            className={`text-xs px-2 py-0.5 rounded ${
              liked
                ? "bg-red-900 text-red-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {liked ? "Liked" : "Like"}
          </button>
          <button
            onClick={() => toggle("favorited")}
            className={`text-xs px-2 py-0.5 rounded ${
              favorited
                ? "bg-yellow-900 text-yellow-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {favorited ? "Favorited" : "Favorite"}
          </button>
          <button
            onClick={() => toggle("watch_later")}
            className={`text-xs px-2 py-0.5 rounded ${
              watchLater
                ? "bg-blue-900 text-blue-300"
                : "bg-neutral-800 text-neutral-400"
            }`}
          >
            {watchLater ? "In Queue" : "Watch Later"}
          </button>
        </div>
      </div>
    </div>
  );
}
