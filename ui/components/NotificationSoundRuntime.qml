import QtQuick
import QtMultimedia

// The actual sound effect engine a notification sound plays through,
// kept in its own file because QtMultimedia is an optional system
// dependency (not every Omarchy install has it): Service.qml loads
// this through a Loader and reads Loader.status to tell whether it is
// there at all, the same reason VoiceNoteRuntime.qml exists apart from
// VoiceNoteController. Importing QtMultimedia anywhere always-loaded
// would make the whole UI fail to load on a machine without it.
Item {
  id: root

  // play starts url playing at once, replacing whatever this effect
  // was already playing: a notification sound is a transient hint,
  // never something worth queuing behind another one.
  function play(url: string): void {
    effect.source = url;
    effect.play();
  }

  SoundEffect {
    id: effect
  }
}
