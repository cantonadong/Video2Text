# Video2Text

Native Windows desktop tool written in Go. It does not use a browser, WebView, Python Whisper, or the OpenAI network API.

## Runtime

The app uses:

- `ffmpeg` to extract mono 16 kHz WAV audio from the selected media file.
- `whisper.cpp` to transcribe that WAV locally.

Default whisper.cpp location:

```text
D:\Models\asr\whisper.cpp
```

Default FFmpeg download location:

```text
D:\Tools\ffmpeg
```

The app searches these files by default:

- `D:\Models\asr\whisper.cpp\Release\whisper-cli.exe`
- `D:\Models\asr\whisper.cpp\Release\main.exe`
- `D:\Models\asr\whisper.cpp\ggml-small.bin`
- `D:\Models\asr\whisper.cpp\ggml-medium.bin`

Existing `.en.bin` models are English-only. To automatically output Chinese or English based on the actual audio language, use a multilingual model such as `ggml-small.bin`.

## Output

Supported input formats include video files (`mp4`, `mkv`, `flv`, `mov`, `avi`, `webm`) and audio files (`mp3`, `ogg`, `aac`).

The transcript is saved next to the source media file:

```text
video.mp4 -> video.txt
meeting.ogg -> meeting.txt
```

## Optional Environment Variables

- `FFMPEG_CMD`: full path to `ffmpeg.exe`
- `WHISPER_CPP_EXE`: full path to `whisper-cli.exe` or `main.exe`
- `WHISPER_MODEL`: full path to a `.bin` whisper.cpp model

The app can download FFmpeg and whisper.cpp from the UI. After a successful download it writes user-level environment variables with `setx`.

## Build

```powershell
powershell -ExecutionPolicy Bypass -File .\build.ps1
```
