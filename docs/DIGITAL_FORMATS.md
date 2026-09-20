# Sampled YM formats

YMT1/YMT2 and MIX1 are now accepted in addition to register-based YM2–YM6.
The loader follows the format definitions in the original
[ST-Sound Ymload.cpp](https://github.com/arnaud-carre/StSound/blob/master/StSoundLibrary/Ymload.cpp).
Playback uses the existing native Go tracker and digi-mix renderers; no SNDH or
68000 emulation is introduced.

YMT streams validate channel counts, frame rates, sample data and pattern
references before playback. The tracker deinterleaver now advances by a complete
channel byte-plane. MIX1 validates sample block boundaries and clears the unfilled
tail of the final output buffer. Input buffers remain caller-owned and unchanged.

The Knucklebuster fixture is the user-selected `Cuddly Knuckle Buster.ym` from
`ST digits complete.ym/ST digit tracks.ym`. Its title is
`KnuckleBusters DigiSynth (from Cuddly Demos)`, its format is YMT1 and its declared
duration is 1,095,000 ms. It is retained here for replay regression testing.

Validation covers exact synthetic PCM values, interleaved/non-interleaved tracker
equivalence, truncated input rejection, end padding and identical real-track
output with large versus irregular consumer blocks. Existing register-emulation
regressions and package race tests continue to pass.

Run `go test -race ./pkg/...`.
