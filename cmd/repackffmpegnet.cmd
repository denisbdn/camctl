ffmpeg -re -i "{{.URLIn}}" -c:v copy -c:a copy -hls_segment_type mpegts -hls_flags independent_segments -hls_flags delete_segments -hls_list_size {{.StorageChanks}} -hls_delete_threshold 6 -strftime 1 -hls_segment_filename "{{.URLOut}}/out_%04Y.%02m.%02d_%02H:%02M:%02S.ts" -hls_time {{.ChankDuration}} -http_persistent 1 -timeout 3.0 -ignore_io_errors 1 "{{.URLOut}}/out.m3u8"