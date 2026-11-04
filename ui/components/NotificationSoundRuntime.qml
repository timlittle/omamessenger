import QtQuick
import QtMultimedia

// Plays notification sounds. Its own file, loaded through a Loader, because
// QtMultimedia is optional and importing it elsewhere would break the UI without it.
Item {
  id: root

  // play plays url at once, replacing any sound already playing.
  function play(url: string): void {
    effect.source = url;
    effect.play();
  }

  SoundEffect {
    id: effect
  }
}
