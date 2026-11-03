"""Create three original 36-second synthetic vowel-humming demo tracks.

This is DSP voice-like synthesis, not a recording or a singing-model output.
Requires only NumPy. Run from any directory; writes web/assets/*.wav.
"""
from pathlib import Path
import wave
import numpy as np

RATE = 24000
DURATION = 36.0
OUT = Path(__file__).resolve().parents[1] / "web" / "assets"
RNG = np.random.default_rng(20261102)


def freq(midi):
    return 440.0 * 2 ** ((midi - 69) / 12)


def soft_noise(n):
    noise = RNG.normal(0, 1, n)
    spec = np.fft.rfft(noise)
    f = np.fft.rfftfreq(n, 1 / RATE)
    spec *= np.exp(-((f - 2600) / 1700) ** 2)
    return np.fft.irfft(spec, n)


def syllable(midi, length, vowel, previous=None):
    n = int(length * RATE)
    t = np.arange(n) / RATE
    f0 = np.full(n, freq(midi))
    # Brief legato onset, slow pitch drift and delayed singing vibrato.
    if previous is not None:
        f0 += (freq(previous) - freq(midi)) * np.exp(-t / 0.045)
    vib_depth = 0.0042 * (1 - np.exp(-t / 0.2))
    f0 *= 1 + vib_depth * np.sin(2 * np.pi * 5.1 * t)
    f0 *= 1 + 0.0008 * np.sin(2 * np.pi * 0.63 * t + midi)
    phase = np.cumsum(f0) * (2 * np.pi / RATE)
    # Voiced harmonics shaped by three moving vowel resonances. The first
    # values approximate /u/ and the second a soft open /a/ vowel.
    if vowel == "u":
        formants = [(360, 135, 1.1), (850, 180, 0.72), (2350, 320, 0.18)]
    else:
        formants = [(720, 150, 1.1), (1150, 220, 0.8), (2550, 350, 0.22)]
    voice = np.zeros(n)
    base_f = freq(midi)
    for harmonic in range(1, min(44, int(11000 / base_f))):
        hz = base_f * harmonic
        weight = 0.035
        for center, width, gain in formants:
            weight += gain * np.exp(-0.5 * ((hz - center) / width) ** 2)
        weight /= harmonic ** 0.62
        voice += weight * np.sin(harmonic * phase + 0.16 * harmonic)
    voice /= max(0.1, np.sqrt(np.mean(voice * voice)))
    # Rounded breath envelope and a gentle consonant-like opening for 'la'.
    attack = 1 - np.exp(-t / 0.075)
    release = np.clip((length - t) / 0.13, 0, 1) ** 1.3
    envelope = attack * release * (0.94 + 0.06 * np.sin(np.pi * t / length))
    if vowel == "a":
        envelope *= 0.72 + 0.28 * (1 - np.exp(-t / 0.055))
    breath = soft_noise(n) * 0.055 * (0.5 + 0.5 * np.exp(-t / 0.1))
    return (voice + breath) * envelope * 0.105


def add_segment(target, sound, start):
    start = int(start * RATE)
    end = min(len(target), start + len(sound))
    if end > start:
        target[start:end] += sound[:end - start]


def chord_pad(notes, length):
    t = np.arange(int(length * RATE)) / RATE
    pad = np.zeros(len(t))
    env = (1 - np.exp(-t / 0.8)) * np.clip((length - t) / 1.3, 0, 1)
    for note in notes:
        f = freq(note)
        pad += np.sin(2 * np.pi * f * t)
        pad += 0.22 * np.sin(2 * np.pi * f * 2.001 * t)
    return pad * env * 0.007


def make_track(filename, phrases, chords, alternate=False):
    voice = np.zeros(int(DURATION * RATE))
    backing = np.zeros_like(voice)
    cursor = 1.0
    previous = None
    for phrase_index, phrase in enumerate(phrases):
        add_segment(backing, chord_pad(chords[phrase_index], 8.8), cursor - 0.5)
        for note_index, (midi, beats) in enumerate(phrase):
            # A short separation between syllables keeps the melody legible.
            vowel = "a" if alternate and note_index % 3 == 1 else "u"
            length = beats - 0.075
            add_segment(voice, syllable(midi, length, vowel, previous), cursor)
            previous = midi
            cursor += beats
        breath = soft_noise(int(0.22 * RATE)) * np.hanning(int(0.22 * RATE)) * 0.01
        add_segment(voice, breath, cursor - 0.25)
    # A restrained stereo room tail without hiding the synthetic vocal.
    left = voice + backing
    right = voice + backing
    for delay, gain in [(0.11, 0.08), (0.23, 0.05), (0.37, 0.025)]:
        offset = int(delay * RATE)
        left[offset:] += voice[:-offset] * gain
        right[offset + 93:] += voice[:-(offset + 93)] * gain
    stereo = np.column_stack((left, right))
    fade = np.minimum(1, np.arange(len(voice)) / (RATE * 0.5))
    fade *= np.clip((len(voice) - np.arange(len(voice))) / (RATE * 2.0), 0, 1)
    stereo *= fade[:, None]
    peak = np.max(np.abs(stereo))
    if peak > 0.88:
        stereo *= 0.88 / peak
    pcm = np.round(np.clip(stereo, -1, 1) * 32767).astype("<i2")
    OUT.mkdir(parents=True, exist_ok=True)
    with wave.open(str(OUT / filename), "wb") as wav:
        wav.setnchannels(2)
        wav.setsampwidth(2)
        wav.setframerate(RATE)
        wav.writeframes(pcm.tobytes())
    print(f"{filename}: {len(pcm) / RATE:.2f}s, stereo PCM16, {RATE} Hz, peak {np.max(np.abs(stereo)):.3f}")


if __name__ == "__main__":
    make_track("tide.wav", [
        [(60, 1), (64, 1), (67, 2), (69, 1), (67, 1), (64, 2)],
        [(62, 1), (64, 1), (67, 1), (64, 1), (62, 2), (60, 2)],
        [(64, 1), (67, 1), (69, 2), (72, 1), (69, 1), (67, 2)],
        [(64, 1), (62, 1), (60, 2), (62, 1), (64, 1), (60, 2)],
    ], [(48, 55, 60), (53, 57, 60), (45, 52, 57), (48, 55, 60)])
    make_track("moon.wav", [
        [(57, 2), (60, 1), (64, 1), (62, 2), (60, 2)],
        [(59, 1), (62, 1), (64, 2), (67, 2), (64, 2)],
        [(65, 1), (64, 1), (62, 2), (60, 1), (62, 1), (64, 2)],
        [(62, 2), (60, 1), (59, 1), (57, 4)],
    ], [(45, 52, 57), (43, 50, 55), (41, 48, 53), (45, 52, 57)])
    make_track("cloud.wav", [
        [(65, 1), (69, 1), (72, 1), (69, 1), (67, 2), (65, 2)],
        [(67, 1), (69, 1), (72, 2), (74, 1), (72, 1), (69, 2)],
        [(69, 1), (67, 1), (65, 1), (64, 1), (62, 2), (65, 2)],
        [(67, 1), (69, 1), (67, 2), (64, 1), (65, 3)],
    ], [(53, 60, 65), (48, 55, 60), (50, 57, 62), (53, 60, 65)], alternate=True)
