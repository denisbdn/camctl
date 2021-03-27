-use_wallclock_as_timestamps 1 -analyzeduration 5M -probesize 5M -fflags +genpts -skip_initial_bytes 1 -vcodec h264 -protocol_whitelist file,rtp,udp -i {{.SdpPath}} -f lavfi -i sine -max_muxing_queue_size 9999 -filter:v scale=-2:720 -filter:a aresample=async=1000 -r 24 -pix_fmt yuv420p -profile:v baseline -vcodec h264_nvenc -acodec aac -b:a 128k -use_timeline 0 -utc_timing_url https://time.akamai.com/?iso -frag_type duration -g:v 12 -keyint_min:v 12 -sc_threshold:v 0 -ldash 1 -tune zerolatency -export_side_data prft -write_prft 0 -target_latency 1.5 -seg_duration 1.0 -frag_duration 1.0 -use_template 1 -index_correction 1 -format_options movflags=cmaf -window_size 4 -extra_window_size 20 -streaming 1 -dash_segment_type mp4 -min_playback_rate 0.8 -max_playback_rate 1.2 -minimum_update_period 0.5 -ldash 1 -init_seg_name {{.InitSegment}}$RepresentationID$.$ext$ -f dash -hls_playlist 1 -strict experimental -lhls 1 -master_pl_name master.m3u8 -method PUT -timeout 0.4 -http_persistent 1 -ignore_io_errors 1 http://127.0.0.1:{{.Port}}/put/{{.Name}}/master.mpd