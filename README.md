Этот сервис позволяет вам сделать собственную систему видеонаблюдения.

Обычно камеры могут передавать поток в формате rtcp, но он внутри использует udp.

Udp нехорошо посылать дальше пределов локальной сети: могут быть как потери,

так и в целом запрет со стороны провайдера. Поэтому реализовал следующую схему

 - У вас стоит камера например dahua DH-IPC-HDW2831TM-AS-S2

 - Она подключена к роутеру (вы настроили роутер так, чтобы можно было обращаться к камере)

 - У вас стоит домашний компьютер/сервер он подключен к тому же роутеру


Если вы имеете статический IP адрес

 - Вы Можете пробросить http/https порт (80/433) из вашего роутера подключенного к этому адресу на ваш сервер


В результате у вас есть запрос на мета описатель потока и чанки которые генерирует ffmpeg и отдает сервер


В системе должен быть установлен ffmpeg, именно он транскодирует rtcp поток в hls/dash поток

ffmpeg -version

ffmpeg version 4.3.1-8ubuntu1~20.04.sav1 Copyright (c) 2000-2020 the FFmpeg developers

для линукс смотрим

https://launchpad.net/~savoury1/+archive/ubuntu/ffmpeg4


сначала компиляция

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


camctl это сервис написанный на go он выступает в 2-х ролях

1 запускает ffmpeg который стучиться за rtcp потоком на камеру

2 принимает, хранит и отдает чанки которые хранит ffmpeg.


Замечания

1 внимательно смотрите права которые есть на роутере как для камеры так и для домашнего сервера

2 тестировал строго на 1-й модели ip камеры, другие камеры могут иметь иные адреса для получения rtcp потока

3 если я включал vpn на сервере видео поток отваливался

4 вы можете настроить запуск ffmpeg на gpu nvidia тогда транскодинг видео не будет занимать cpu вашего сервера
