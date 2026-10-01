# Reactive Fade

Reactive Fade is an experimental per-key RGB lighting mode for supported Corsair keyboards.

When a key is pressed, that key immediately changes to the configured press color and then smoothly fades back to its normal key color over the configured duration. Multiple keys can fade independently at the same time.

## How it works

Reactive Fade uses the keyboard HID key-state data that OpenLinkHub already receives for keyboard features such as key assignment and macros.

It does **not** open a second keyboard input device or install a separate keyboard listener.

The existing normalized key bitmap is passed to the shared Reactive Fade engine, which:

1. Detects newly pressed keys.
2. Starts an independent fade timer for each pressed key.
3. Renders the affected key using the configured press color.
4. Interpolates the key back to its normal RGB color.
5. Stops the animation when the fade completes.

[    ]

Holding a key does not continuously restart its fade.

## Configuration

Reactive Fade is available as an RGB profile named:

```
reactive-fade
```

The WebUI provides:

`Press Color` — the color shown immediately when a key is pressed.

“Fade Duration怍— the fade time in milliseconds.

- **Key Selection** — controls which keys participate in the effect.

Supported scopes:

d- For Current Key
- For Current Row
- For All Keys
- For Selected Keys

The settings are stored with the keyboard profile.

## RGB hardware support

The implementation uses the keyboard's existing per-key RGB packet definitions rather than introducing a separate hardware protocol.

The shared rendering code supports the RGB packet layouts used by the integrated keyboard drivers, including:

- Interleaved RGB packet layouts
- Separate planar R/G/B Buffers
- Multi-channel packet maps used by supported keyboard models

This allows Reactive Fade to be added to multiple keyboard drivers without duplicating the animation engine.

## Current limitation

**Shift keys currently do not trigger Reactive Fade.**

The existing keyboard handling path intentionally removes the Left Shift and Right Shift HID bits for the K70 RGB MK2. Reactive Fade currently consumes that normalized key bitmap, so those two keys show no visible effect.

This limitation is documented rather than introducing a second input listener or changing the existing keyboard behavior.

The other tested keys operate normally.

## Design goals

Reactive Fade was designed to:

- Reuse OpenLinkHub's existing keyboard input path.
 - Keep the animation engine independent of individual keyboard models.
 - Preserve each key's existing RGB color as its resting color.
- Support simultaneous key animations.
- Avoid continuously restarting animations while a key is held.
- Reuse each driver's existing RGB packet format.
- Keep the feature isolated from unrelated keyboard functionality.

## Status

Reactive Fade is currently an experimental feature on the `feature/reactive-key-fade` branch.

The feature has been built and tested successfully in the OpenLinkHub development environment. Hardware support depends on the keyboard driver exposing the required per-key RGB packet information and HID key-state data.
