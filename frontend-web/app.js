const callBtn = document.getElementById("callBtn");
const statusEl = document.getElementById("status");
const videosEl = document.getElementById("videos");

let room;

callBtn.addEventListener("click", async () => {
  try {
    statusEl.innerText = "Calling API...";

    // 1. Call backend
    const callerId = `user-${Date.now()}`; // Generate unique caller ID
    const res = await fetch("http://localhost:6060/calls", {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify({ caller_id: callerId })
    });

    if (!res.ok) {
      throw new Error("Failed to call API");
    }

    const data = await res.json();
    console.log("API response:", data);

    const { call_id, session_id, token, url } = data;

    statusEl.innerText = `Joining room: ${call_id}`;

    // 2. Join LiveKit
    room = new LivekitClient.Room({
      adaptiveStream: true,
      dynacast: true,
    });

    // Event ketika participant publish track
    room.on(
      LivekitClient.RoomEvent.TrackSubscribed,
      (track, publication, participant) => {
        if (track.kind === "video") {
          const videoEl = track.attach();
          videosEl.appendChild(videoEl);
        }
        if (track.kind === "audio") {
          track.attach();
        }
      }
    );

    // 3. Connect
    await room.connect(url, token);

    statusEl.innerText = `Connected as ${session_id}`;

    // 4. Publish local media
    await room.localParticipant.enableCameraAndMicrophone();

    // Tampilkan video lokal
    room.localParticipant.videoTracks.forEach((pub) => {
      const videoEl = pub.track.attach();
      videosEl.appendChild(videoEl);
    });

  } catch (err) {
    console.error(err);
    statusEl.innerText = "Error: " + err.message;
  }
});
