# iosscreenshotmaker

Create media from ios simulator captures and html based templates.
- screenshot for the appstore
- videos for social media

## Prerequisites
- Works on mac
- `ffmpeg` for video framing, I installed it via brew
```sh
ffmpeg -version
ffmpeg version 8.1 Copyright (c) 2000-2026 the FFmpeg developers
```

- A Chromium-compatible browser for rendering and capturing screenshot layouts.
```sh
/Applications/Chromium.app/Contents/MacOS/Chromium
```

I installed

```sh
brew install --cask ungoogled-chromium
```

# Getting started

```sh
go build -o iosscreenshotmaker main.go
```

## Capture screenshot
- Assumes simulator running and is an iphone (I used iphone 17)
```sh
mkdir -p example/play-along
xcrun simctl io booted screenshot "$PWD/example/play-along/screenshot.png"
```

## Frame it
```sh
./iosscreenshotmaker frame-screenshot \
--frame "frames/iPhone Air - Cloud White - Portrait.png" \
--input "example/play-along/screenshot.png" \
--output "example/play-along/framed.png"
```

## Add frame to template
### Start the web server
- We use this to render the image in the template
```sh
./iosscreenshotmaker server ./example/www
```

### Add frame to template
```sh
jq -n \
--arg input "example/play-along/framed.png" \
--arg output "example/play-along/framed-with-template.png" \
--arg screenshot_bg_color "#000" \
--arg title_color "#00ff00" \
--arg title "Hello World" \
'
{
    input: $input,
    output: $output,
    template: {
        title: $title,
        title_color: $title_color,
        screenshot_bg_color: $screenshot_bg_color
    }
}' | ./iosscreenshotmaker create-app-store-screenshot \
--chrome="/Applications/Chromium.app/Contents/MacOS/Chromium" \
--device="iphone" \
--server-dir "example/www"
```

### Open the debug view
- Running the above, should have written to the terminal
```sh
open_url: http://127.0.0.1:8080/index.html?capture=0&config=%7B%22height%22%3A2778%2C%22imageUri%22%3A%22%2Finput%2F30bcd266abae9f0a684b3a21%2Fframed.png%22%2C%22input%22%3A%22example%2Fplay-along%2Fframed.png%22%2C%22name%22%3A%22framed.png%22%2C%22output%22%3A%22example%2Fplay-along%2Fframed-with-template.png%22%2C%22template%22%3A%7B%22screenshot_bg_color%22%3A%22%23F5F1E8%22%2C%22title%22%3A%22Hello+World%22%2C%22title_color%22%3A%22%230F172A%22%7D%2C%22width%22%3A1284%7D
```

- if you open the url, in your browser, you will see your image + the template
- the template is html + css (tailwind), look in [example/www/index.html](example/www/index.html)


# Reference
- https://github.com/ungoogled-software/ungoogled-chromium#downloads
- https://tailwindcss.com/
