import { useState } from "react";
import { resolveChannel, createRecord } from "./api";
import { v4 as uuidv4 } from "uuid";

export default function AddChannel({ onAdded }) {
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleAdd(e) {
    e.preventDefault();
    if (!url.trim()) return;

    setLoading(true);
    setError("");

    try {
      const info = await resolveChannel(url.trim());
      await createRecord("channels", {
        id: uuidv4(),
        channel_id: info.channel_id,
        channel_name: info.channel_name,
        rss_url: info.rss_url,
      });
      setUrl("");
      setOpen(false);
      onAdded?.();
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="w-full mb-4 px-3 py-2 border border-dashed border-neutral-700 rounded-lg text-sm text-neutral-400 hover:text-neutral-200 hover:border-neutral-500 cursor-pointer"
      >
        + Add channel
      </button>

      {open && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
          onClick={(e) => { if (e.target === e.currentTarget) setOpen(false); }}
        >
          <div className="bg-neutral-900 border border-neutral-700 rounded-xl p-6 w-96 shadow-xl">
            <h2 className="text-sm font-semibold text-neutral-100 mb-4">Add Channel</h2>
            <form onSubmit={handleAdd} className="space-y-3">
              <input
                type="text"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="youtube.com/@Channel"
                autoFocus
                className="w-full px-3 py-2 bg-neutral-800 border border-neutral-700 text-neutral-100 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-red-500 placeholder-neutral-500"
                disabled={loading}
              />
              {error && <p className="text-red-400 text-xs">{error}</p>}
              <div className="flex gap-2 justify-end">
                <button
                  type="button"
                  onClick={() => { setOpen(false); setError(""); setUrl(""); }}
                  className="px-4 py-2 text-sm text-neutral-400 hover:text-neutral-200 cursor-pointer"
                  disabled={loading}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={loading || !url.trim()}
                  className="px-4 py-2 bg-red-600 text-white rounded-lg text-sm font-medium hover:bg-red-700 disabled:opacity-50 cursor-pointer"
                >
                  {loading ? "Adding..." : "Add"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
