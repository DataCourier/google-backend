import { useState, useEffect, useCallback } from "react";
import { Routes, Route, useNavigate, useParams, useLocation } from "react-router-dom";
import AddChannel from "./AddChannel";
import ChannelList from "./ChannelList";
import Feed from "./Feed";
import VideoPage from "./VideoPage";
import LoginPage from "./LoginPage";
import { listRecords, fetchRSS, fetchAllVideos, batchRecords, isLoggedIn, logout, setOnUnauthorized } from "./api";
import { v4 as uuidv4 } from "uuid";

function App() {
  const [loggedIn, setLoggedIn] = useState(isLoggedIn());

  useEffect(() => {
    setOnUnauthorized(() => setLoggedIn(false));
  }, []);

  const [channels, setChannels] = useState([]);
  const [videos, setVideos] = useState([]);
  const [refreshing, setRefreshing] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();

  const loadChannels = useCallback(async () => {
    const data = await listRecords("channels");
    setChannels(data || []);
    return data || [];
  }, []);

  const loadVideos = useCallback(async () => {
    const data = await listRecords("videos");
    setVideos(data || []);
  }, []);

  const refreshFeeds = useCallback(
    async (channelList) => {
      setRefreshing(true);
      try {
        const existingIds = new Set(videos.map((v) => v.video_id));
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
          await batchRecords("videos", newVideos);
        }

        await loadVideos();
        localStorage.setItem("last_refresh_date", todayStr());
      } finally {
        setRefreshing(false);
      }
    },
    [videos, loadVideos]
  );

  useEffect(() => {
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
            await batchRecords("videos", newVideos);
            await loadVideos();
          }
          localStorage.setItem("last_refresh_date", todayStr());
        } finally {
          setRefreshing(false);
        }
      }
    }
    init();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  function todayStr() {
    return new Date().toISOString().slice(0, 10);
  }

  // Derive selected state from URL
  const path = location.pathname;
  let selectedChannel = null;
  if (path === "/liked") selectedChannel = "liked";
  else if (path === "/watch-later") selectedChannel = "watch_later";
  else if (path.startsWith("/channel/")) selectedChannel = path.slice("/channel/".length);

  function handleSelectChannel(channelId) {
    if (!channelId) navigate("/");
    else navigate(`/channel/${channelId}`);
  }

  if (!loggedIn) {
    return <LoginPage onLogin={() => setLoggedIn(true)} />;
  }

  return (
    <div className="flex h-screen bg-neutral-950 text-neutral-100">
      {/* Sidebar */}
      <div className="w-64 border-r border-neutral-800 p-4 flex flex-col">
        <h1
          className="text-lg font-bold text-red-500 mb-4 cursor-pointer"
          onClick={() => navigate("/")}
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
                const existingIds = new Set(videos.map((v) => v.video_id));
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
                  await batchRecords("videos", newVids);
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
                await batchRecords("videos", newVids.slice(i, i + 100));
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
            onClick={() => navigate("/liked")}
            className={`w-full text-left px-3 py-2 rounded text-sm cursor-pointer ${
              selectedChannel === "liked"
                ? "bg-red-900/50 text-red-300 font-medium"
                : "hover:bg-neutral-800 text-neutral-300"
            }`}
          >
            Liked ({videos.filter((v) => v.liked).length})
          </button>
          <button
            onClick={() => navigate("/watch-later")}
            className={`w-full text-left px-3 py-2 rounded text-sm cursor-pointer ${
              selectedChannel === "watch_later"
                ? "bg-blue-900/50 text-blue-300 font-medium"
                : "hover:bg-neutral-800 text-neutral-300"
            }`}
          >
            Watch Later ({videos.filter((v) => v.watch_later).length})
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
          <button
            onClick={async () => {
              await logout();
              setLoggedIn(false);
            }}
            className="w-full text-xs px-3 py-2 text-neutral-500 hover:text-neutral-300 hover:bg-neutral-800 rounded"
          >
            Logout
          </button>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 p-6 overflow-y-auto bg-neutral-950">
        <Routes>
          <Route
            path="/video/:videoId"
            element={
              <VideoPageWrapper
                videos={videos}
                loadVideos={loadVideos}
              />
            }
          />
          <Route
            path="*"
            element={
              <>
                {refreshing && (
                  <div className="mb-4 text-sm text-neutral-400">Refreshing feeds...</div>
                )}
                <Feed
                  videos={videos}
                  selectedChannel={selectedChannel}
                  onUpdated={loadVideos}
                  onSelectVideo={(v) => navigate(`/video/${v.video_id}`)}
                />
              </>
            }
          />
        </Routes>
      </div>
    </div>
  );
}

function VideoPageWrapper({ videos, loadVideos }) {
  const { videoId } = useParams();
  const navigate = useNavigate();
  const video = videos.find((v) => v.video_id === videoId);

  if (!video) {
    return <div className="text-neutral-400 text-sm">Video not found</div>;
  }

  return (
    <VideoPage
      video={video}
      onBack={() => navigate(-1)}
      onUpdated={loadVideos}
    />
  );
}

export default App;
