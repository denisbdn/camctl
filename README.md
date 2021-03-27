в системе должен быть утсановлен ffmpeg
ffmpeg -version
ffmpeg version 4.3.1-8ubuntu1~20.04.sav1 Copyright (c) 2000-2020 the FFmpeg developers

https://launchpad.net/~savoury1/+archive/ubuntu/ffmpeg4


сначала

make
sudo make install


потом добавляем сервис в systemd
sudo mcedid /lib/systemd/system/camctl.service
[Unit]
Description=RTSP Group
After=network.target
After=network-online.target

[Service]
Type=exec
ExecStart=/usr/local/bin/camctl -addr :6060 -tmpl /usr/local/etc/camctl/tmpl/ -cmd /usr/local/etc/camctl/cmd/ -static /usr/local/etc/camctl/static/ -workDir /tmpl/camctl/ffmpeg/

[Install]
WantedBy=multi-user.target


потом обновляем
sudo systemctl daemon-reload


запускаем
sudo systemctl start camctl.service


если все успешно стартуем
sudo systemctl start camctl.service 



