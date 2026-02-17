import { useState, useEffect, useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import AddChannel from "./AddChannel";
import ChannelList from "./ChannelList";
import Feed from "./Feed";
import VideoPage from "./VideoPage";
import LoginPage from "./LoginPage";
import { listRecords, fetchRSS, fetchAllVideos, batchRecords, isLoggedIn, logout, setOnUnauthorized, getEmail } from "./api";
import { v4 as uuidv4 } from "uuid";

function App() {
  const [loggedIn, setLoggedIn] = useState(isLoggedIn());

  useEffect(() => {
    setOnUnauthorized(() => setLoggedIn(false));
  }, []);

  const [channels, setChannels] = useState([]);
  const [videos, setVideos] = useState([]);
  const [videosTotal, setVideosTotal] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const [scrollToVideoId, setScrollToVideoId] = useState(null);
  const [singleVideo, setSingleVideo] = useState(null);
  const [singleVideoLoading, setSingleVideoLoading] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();

  const page = searchParams.get("p") || "";

  function nav(p) {
    if (p) {
      setSearchParams({ p });
    } else {
      setSearchParams({});
    }
  }

  const loadChannels = useCallback(async () => {
    const data = await listRecords("channels");
    setChannels(data || []);
    return data || [];
  }, []);

  const PAGE_SIZE = 100;

  const loadVideos = useCallback(async (append = false, offset = 0) => {
    const result = await listRecords("videos", {
      limit: PAGE_SIZE,
      offset,
      orderBy: "published",
      orderDir: "desc",
    });
    if (append) {
      setVideos((prev) => [...prev, ...result.data]);
    } else {
      setVideos(result.data);
    }
    setVideosTotal(result.total);
  }, []);

  const loadMoreVideos = useCallback(async () => {
    await loadVideos(true, videos.length);
  }, [loadVideos, videos.length]);

  const refreshFeeds = useCallback(
    async (channelList) => {
      setRefreshing(true);
      try {
        const allVids = await listRecords("videos");
        const existingIds = new Set(allVids.map((v) => v.video_id));
        let newVideos = [];

        for (const ch of channelList) {
          try {
            const feed = await fetchRSS(ch.channel_id);
            for (const v of feed.videos || []) {
              if (!existingIds.has(v.video_id)) {
                existingIds.add(v.video_id);
                newVideos.push({
                  id: uuidv4(),
                  video_id: v.video_id,
                  title: v.title,
                  published: v.published,
                  channel: v.channel,
                  channel_id: ch.channel_id,
                  thumbnail: v.thumbnail,
                  description: v.description,
                  watched: false,
                  favorited: false,
                });
              }
            }
          } catch (err) {
            console.error(`Failed to fetch RSS for ${ch.channel_name}:`, err);
          }
        }

        if (newVideos.length > 0) {
          await batchRecords("videos", newVideos, { dedupeOn: "video_id" });
        }

        await loadVideos();
        localStorage.setItem("last_refresh_date", todayStr());
      } finally {
        setRefreshing(false);
      }
    },
    [loadVideos]
  );

  useEffect(() => {
    if (!loggedIn) return;
    async function init() {
      const chs = await loadChannels();
      await loadVideos();

      const lastRefresh = localStorage.getItem("last_refresh_date");
      if (lastRefresh !== todayStr() && chs.length > 0) {
        const existingVids = await listRecords("videos");
        setVideos(existingVids || []);
        setRefreshing(true);
        try {
          const existingIds = new Set((existingVids || []).map((v) => v.video_id));
          let newVideos = [];
          for (const ch of chs) {
            try {
              const feed = await fetchRSS(ch.channel_id);
              for (const v of feed.videos || []) {
                if (!existingIds.has(v.video_id)) {
                  existingIds.add(v.video_id);
                  newVideos.push({
                    id: uuidv4(),
                    video_id: v.video_id,
                    title: v.title,
                    published: v.published,
                    channel: v.channel,
                    channel_id: ch.channel_id,
                    thumbnail: v.thumbnail,
                    description: v.description,
                    watched: false,
                    favorited: false,
                  });
                }
              }
            } catch (err) {
              console.error(`Failed to fetch RSS for ${ch.channel_name}:`, err);
            }
          }
          if (newVideos.length > 0) {
            await batchRecords("videos", newVideos, { dedupeOn: "video_id" });
            await loadVideos();
          }
          localStorage.setItem("last_refresh_date", todayStr());
        } finally {
          setRefreshing(false);
        }
      }
    }
    init();
  }, [loggedIn]); // eslint-disable-line react-hooks/exhaustive-deps

  function todayStr() {
    return new Date().toISOString().slice(0, 10);
  }

  // Derive selected state from ?p= param
  let selectedChannel = null;
  if (page === "liked") selectedChannel = "liked";
  else if (page === "watch-later") selectedChannel = "watch_later";
  else if (page === "notes") selectedChannel = "notes";
  else if (page.startsWith("channel/")) selectedChannel = page.slice("channel/".length);

  function handleSelectChannel(channelId) {
    if (!channelId) nav("");
    else nav(`channel/${channelId}`);
  }

  useEffect(() => {
    if (!page.startsWith("video/")) {
      setSingleVideo(null);
      return;
    }
    const videoId = page.slice("video/".length);
    const found = videos.find((v) => v.video_id === videoId);
    if (found) {
      setSingleVideo(found);
    } else {
      setSingleVideoLoading(true);
      listRecords("videos", { limit: 1, filters: { video_id: videoId } })
        .then((res) => {
          setSingleVideo(res.data?.[0] || null);
          setSingleVideoLoading(false);
        })
        .catch(() => setSingleVideoLoading(false));
    }
  }, [page, videos]);

  if (!loggedIn) {
    return <LoginPage onLogin={() => setLoggedIn(true)} />;
  }

  if (page.startsWith("video/")) {
    const videoId = page.slice("video/".length);
    const video = videos.find((v) => v.video_id === videoId) || singleVideo;
    if (!video) {
      return (
        <div className="flex h-screen bg-neutral-950 text-neutral-100 items-center justify-center">
          <div className="text-neutral-400 text-sm">{singleVideoLoading ? "Loading..." : "Video not found"}</div>
        </div>
      );
    }
    return (
      <div className="flex h-screen bg-neutral-950 text-neutral-100">
        <div className="flex-1 p-6 overflow-y-auto">
          <VideoPage
            video={video}
            onBack={() => {
              setScrollToVideoId(videoId);
              window.history.back();
            }}
            onUpdated={loadVideos}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-screen bg-neutral-950 text-neutral-100">
      {/* Sidebar */}
      <div className="w-64 border-r border-neutral-800 p-4 flex flex-col">
        <h1
          className="text-lg font-bold text-red-500 mb-4 cursor-pointer"
          onClick={() => nav("")}
        >
          FocusTube
        </h1>
        <AddChannel
          onAdded={async () => {
            const chs = await loadChannels();
            const latest = chs[chs.length - 1];
            if (latest) {
              setRefreshing(true);
              try {
                const feed = await fetchRSS(latest.channel_id);
                const allVids = await listRecords("videos");
                const existingIds = new Set(allVids.map((v) => v.video_id));
                const newVids = (feed.videos || [])
                  .filter((v) => !existingIds.has(v.video_id))
                  .map((v) => ({
                    id: uuidv4(),
                    video_id: v.video_id,
                    title: v.title,
                    published: v.published,
                    channel: v.channel,
                    channel_id: latest.channel_id,
                    thumbnail: v.thumbnail,
                    description: v.description,
                    watched: false,
                    favorited: false,
                  }));
                if (newVids.length > 0) {
                  await batchRecords("videos", newVids, { dedupeOn: "video_id" });
                }
                await loadVideos();
              } finally {
                setRefreshing(false);
              }
            }
          }}
        />
        <ChannelList
          channels={channels}
          selectedId={selectedChannel}
          onSelect={handleSelectChannel}
          onDeleted={loadChannels}
          onFetchAll={async (ch) => {
            setRefreshing(true);
            try {
              const result = await fetchAllVideos(ch.channel_id);
              const existingVids = await listRecords("videos");
              const existingIds = new Set((existingVids || []).map((v) => v.video_id));
              const newVids = (result.videos || [])
                .filter((v) => !existingIds.has(v.video_id))
                .map((v) => ({
                  id: uuidv4(),
                  video_id: v.video_id,
                  title: v.title,
                  published: v.published,
                  channel: v.channel,
                  channel_id: ch.channel_id,
                  thumbnail: v.thumbnail,
                  description: v.description,
                  watched: false,
                  favorited: false,
                }));
              for (let i = 0; i < newVids.length; i += 100) {
                await batchRecords("videos", newVids.slice(i, i + 100), { dedupeOn: "video_id" });
              }
              await loadVideos();
              alert(`Fetched ${result.total} total videos, ${newVids.length} new`);
            } catch (err) {
              alert("Fetch all failed: " + err.message);
            } finally {
              setRefreshing(false);
            }
          }}
        />
        {/* Smart Lists */}
        <div className="mt-4 pt-4 border-t border-neutral-800 space-y-1">
          <button
            onClick={() => nav("liked")}
            className={`w-full text-left px-3 py-2 rounded text-sm cursor-pointer ${
              selectedChannel === "liked"
                ? "bg-red-900/50 text-red-300 font-medium"
                : "hover:bg-neutral-800 text-neutral-300"
            }`}
          >
            Liked ({videos.filter((v) => v.liked).length})
          </button>
          <button
            onClick={() => nav("watch-later")}
            className={`w-full text-left px-3 py-2 rounded text-sm cursor-pointer ${
              selectedChannel === "watch_later"
                ? "bg-blue-900/50 text-blue-300 font-medium"
                : "hover:bg-neutral-800 text-neutral-300"
            }`}
          >
            Watch Later ({videos.filter((v) => v.watch_later).length})
          </button>
          <button
            onClick={() => nav("notes")}
            className={`w-full text-left px-3 py-2 rounded text-sm cursor-pointer ${
              selectedChannel === "notes"
                ? "bg-purple-900/50 text-purple-300 font-medium"
                : "hover:bg-neutral-800 text-neutral-300"
            }`}
          >
            Notes ({videos.filter((v) => v.notes).length})
          </button>
        </div>

        <div className="mt-auto pt-4 space-y-2">
          <button
            onClick={() => refreshFeeds(channels)}
            disabled={refreshing || channels.length === 0}
            className="w-full text-xs px-3 py-2 bg-neutral-800 rounded hover:bg-neutral-700 disabled:opacity-50"
          >
            {refreshing ? "Refreshing..." : "Refresh feeds"}
          </button>
          <div className="flex items-center justify-between">
            <span className="text-xs text-neutral-500 truncate">{getEmail()}</span>
            <button
              onClick={async () => {
                await logout();
                setLoggedIn(false);
              }}
              className="text-xs text-neutral-500 hover:text-neutral-300 shrink-0"
            >
              Logout
            </button>
          </div>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 p-6 overflow-y-auto bg-neutral-950">
        {refreshing && (
          <div className="mb-4 text-sm text-neutral-400">Refreshing feeds...</div>
        )}
        <Feed
          videos={videos}
          selectedChannel={selectedChannel}
          onUpdated={loadVideos}
          onSelectVideo={(v) => nav(`video/${v.video_id}`)}
          scrollToVideoId={scrollToVideoId}
          onScrolled={() => setScrollToVideoId(null)}
        />
        {videos.length < videosTotal && (
          <button
            onClick={loadMoreVideos}
            className="mt-4 w-full py-2 text-sm text-neutral-400 bg-neutral-900 rounded hover:bg-neutral-800"
          >
            Load more ({videos.length} of {videosTotal})
          </button>
        )}
      </div>
    </div>
  );
}

export default App;
