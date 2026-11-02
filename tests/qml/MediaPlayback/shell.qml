// Proves Qt's own QtMultimedia can actually decode and play what
// backend/internal/transcode's Video and Sound profiles write, not only
// that ffprobe can read the container back: sample.mp4 (160x90 H.264 with
// AAC) and sample.wav (mono 16-bit PCM) are both committed fixtures this
// exact pipeline produced once - real sandboxed ffmpeg, then this
// package's own mp4ff remux or WAV rebuild - never regenerated here. No
// helper process is needed: both files are loaded directly from disk.
import QtQuick
import QtMultimedia
import Quickshell
import "Check.js" as Check

ShellRoot {
  id: root

  MediaDevices {
    id: devices
  }

  MediaPlayer {
    id: videoPlayer
    source: Qt.resolvedUrl("sample.mp4")
    audioOutput: AudioOutput { device: devices.defaultAudioOutput }
  }

  MediaPlayer {
    id: audioPlayer
    source: Qt.resolvedUrl("sample.wav")
    audioOutput: AudioOutput { device: devices.defaultAudioOutput }
  }

  Stepper {
    id: stepper
    name: "MediaPlayback"
    deadlineMs: 20000
    onTimeout: () => Check.fail("timed out before both fixtures finished playing")
    startFn: root.start
  }

  // start plays the video fixture first; the audio fixture follows once
  // it is confirmed playing, so a failure names which file is at fault.
  function start(): void {
    videoPlayer.play();
    root.waitForVideoPlaying();
  }

  // waitForVideoPlaying holds until sample.mp4 reaches PlayingState with
  // a known duration and a position that has actually started advancing,
  // not just a state flag flipped with nothing decoding behind it.
  function waitForVideoPlaying(): void {
    if (videoPlayer.playbackState === MediaPlayer.PlayingState && videoPlayer.duration > 0 && videoPlayer.position > 0)
      return root.startAudio();

    stepper.attempts++;
    if (stepper.attempts >= 150) {
      return Check.fail("sample.mp4 never reached PlayingState with an advancing position: state="
        + videoPlayer.playbackState + " duration=" + videoPlayer.duration + " position=" + videoPlayer.position
        + " error=" + videoPlayer.errorString);
    }
    stepper.retry(root.waitForVideoPlaying);
  }

  function startAudio(): void {
    stepper.attempts = 0;
    audioPlayer.play();
    root.waitForAudioPlaying();
  }

  // waitForAudioPlaying is waitForVideoPlaying's own twin for sample.wav.
  function waitForAudioPlaying(): void {
    if (audioPlayer.playbackState === MediaPlayer.PlayingState && audioPlayer.duration > 0 && audioPlayer.position > 0)
      return root.finish();

    stepper.attempts++;
    if (stepper.attempts >= 150) {
      return Check.fail("sample.wav never reached PlayingState with an advancing position: state="
        + audioPlayer.playbackState + " duration=" + audioPlayer.duration + " position=" + audioPlayer.position
        + " error=" + audioPlayer.errorString);
    }
    stepper.retry(root.waitForAudioPlaying);
  }

  function finish(): void {
    console.log("PASS MediaPlayback");
    Qt.exit(0);
  }
}
