import { deleteRecord } from "./api";

export default function ChannelList({ channels, selectedId, onSelect, onDeleted, onFetchAll }) {
  async function handleDelete(e, id) {
    e.stopPropagation();
    await deleteRecord("channels", id);
    onDeleted?.();
  }

  function handleFetchAll(e, ch) {
    e.stopPropagation();
    onFetchAll?.(ch);
  }

  return (
    <div className="space-y-1">
      <button
        onClick={() => onSelect(null)}
        className={`w-full text-left px-3 py-2 rounded text-sm cursor-pointer ${
          !selectedId ? "bg-red-900/50 text-red-300 font-medium" : "hover:bg-neutral-800 text-neutral-300"
        }`}
      >
        All channels
      </button>
      {channels.map((ch) => (
        <div
          key={ch.id}
          onClick={() => onSelect(ch.channel_id)}
          className={`flex items-center justify-between px-3 py-2 rounded text-sm cursor-pointer ${
            selectedId === ch.channel_id
              ? "bg-red-900/50 text-red-300 font-medium"
              : "hover:bg-neutral-800 text-neutral-300"
          }`}
        >
          <span className="truncate flex-1">{ch.channel_name || ch.channel_id}</span>
          <button
            onClick={(e) => handleFetchAll(e, ch)}
            className="text-neutral-500 hover:text-blue-400 ml-1 shrink-0 text-xs"
            title="Fetch all videos (YouTube API)"
          >
            &#8635;
          </button>
          <button
            onClick={(e) => handleDelete(e, ch.id)}
            className="text-neutral-500 hover:text-red-400 ml-1 shrink-0"
            title="Remove channel"
          >
            &times;
          </button>
        </div>
      ))}
    </div>
  );
}
