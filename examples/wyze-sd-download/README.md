# Single-file Wyze SD download example

Experimental integration helper for the native Wyze/RDT client. Build on Linux
with `go build -o wyze-sd-download ./examples/wyze-sd-download`. No service,
archive scheduler, browser UI, cloud login, or firmware change is included.

An integration supplies already cached pairing metadata (`ip`, `uid`, `enr`,
`mac`, `model`) in a private regular JSON file with permissions 0600. It invokes
the binary without a shell:

```
wyze-sd-download --live --credentials /private/camera.json \
  --download /media/mmc/record/20260101/12/34.mp4 \
  --media-output /private/clip.mp4 --output /private/result.json \
  --max-bytes 16777216
```

Pairing input and both outputs require current-user-owned private parent
directories (0700 or stricter). The pairing file must be a regular file with
permissions 0600 or stricter; both outputs must be new files. Result names must
end in `.json`, media names in `.mp4`. Existing files are preserved.
The default limit is 16 MiB, with an absolute maximum of 64 MiB. Only normal
recording paths and their `_new.mp4` variant are accepted. Dates and times are
camera local. Each invocation opens a fresh authenticated session, downloads
one file, closes the session, and exits. Without `--live`, it only validates the
request. An integrating caller must impose a process timeout and delete the
temporary directory after use.

Success requires exit code zero, JSON `operation=download_sd_file` and
`phase=download_complete`, matching
`recording_path`, and `downloaded_bytes` equal to the MP4's actual size. Errors
are fixed local diagnostics; credentials and upstream error text are suppressed.
Failed transfers remove the media output. A caller must reject partial output
and must not expose the pairing JSON or result file through its media server.

This example deliberately retrieves a single clip. It does not enumerate,
retain, or automatically download a recording history.
