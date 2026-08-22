# Installing 3d-printer-monitor on Raspberry Pi

This guide sets up a headless Raspberry Pi from an empty microSD card through
the first successful container start. It includes Raspberry Pi Imager,
Raspberry Pi Connect, SSH, Docker, and automatic container restart.

## Requirements

- Raspberry Pi 3, 4, or 5 with a 64-bit-capable processor
- Reliable power supply suitable for the Pi model
- 16 GB or larger high-quality microSD card
- Ethernet, or the Wi-Fi name and password
- A computer with a microSD card reader
- Raspberry Pi ID for Raspberry Pi Connect
- Telegram bot token and chat ID
- Bambu printer LAN address, serial number, and LAN access code

Use a 64-bit Raspberry Pi OS image. The Docker image and race-enabled Go tests
are intended for `arm64`.

## 1. Write Raspberry Pi OS with Raspberry Pi Imager

1. Download and install [Raspberry Pi Imager](https://www.raspberrypi.com/software/).
2. Insert the microSD card into the computer.
3. Open Imager and select the Raspberry Pi model.
4. Select **Raspberry Pi OS Lite (64-bit)**. The Lite image is recommended for a
   headless service because it does not install a desktop environment.
5. Select the microSD card as storage.
6. Open **OS Customisation** before writing the image.

Configure these fields:

- **Hostname:** for example `3d-printer-monitor`. Use only letters, digits, and
  hyphens.
- **User:** create a lowercase username and a strong, unique password. The
  examples below use `observer`.
- **Wi-Fi:** configure SSID, password, hidden-network setting when applicable,
  and the correct wireless country. Skip Wi-Fi when Ethernet will be used.
- **Locale:** select the correct timezone and keyboard layout.
- **Remote Access:** enable SSH. Public-key authentication is recommended;
  password authentication is acceptable for initial setup.
- **Raspberry Pi Connect (optional):** open this tab and enable **Raspberry Pi Connect**.
  Select **Open Raspberry Pi Connect**, sign in with Raspberry Pi ID, create the
  temporary authentication key, and return to Imager so the key is included in
  the image.

Review the customisation summary and confirm that Raspberry Pi Connect is
enabled. Write the image and wait until Imager verifies it. Eject the card
safely.

Raspberry Pi documents the current Imager customisation and headless workflow in
[Getting started](https://www.raspberrypi.com/documentation/computers/getting-started.html).

## 2. First boot

1. Insert the microSD card into the powered-off Pi.
2. Connect Ethernet when available. A wired connection is preferable for a
   continuously running printer monitor.
3. Connect power.
4. Allow several minutes for storage expansion, first-boot configuration, and
   the initial network connection. The Pi can reboot during this process. Once
   online, it uses the one-time Connect authentication key embedded by Imager
   to link itself to the selected Raspberry Pi ID.

First, open [connect.raspberrypi.com](https://connect.raspberrypi.com/), select
the new device, and verify that remote shell access works.

SSH remains the local fallback. From another computer, connect with the username
selected in Imager:

```bash
ssh observer@3d-printer-monitor.local
```

If mDNS does not resolve the hostname, find the Pi address in the router and use:

```bash
ssh observer@192.168.1.100
```

Confirm that this is the expected host key before accepting it.

## 3. Update and verify the operating system

```bash
sudo apt update
sudo apt full-upgrade -y
sudo reboot
```

Reconnect after the reboot and verify the 64-bit architecture, clock, storage,
and network:

```bash
uname -m
timedatectl
df -h /
hostname -I
ping -c 3 1.1.1.1
```

`uname -m` should report `aarch64`.

Optionally reserve the Pi's address in the router's DHCP settings. A stable
address makes administration easier, although 3d-printer-monitor itself does not
listen on a network port.

## 4. Verify Raspberry Pi Connect (optional)
The Pi should then appear at
[connect.raspberrypi.com](https://connect.raspberrypi.com/). Raspberry Pi
documents linking, remote-shell behavior, and troubleshooting in the official
[Raspberry Pi Connect documentation](https://www.raspberrypi.com/documentation/services/connect.html).

Keep SSH enabled as a local fallback. Raspberry Pi Connect depends on working
internet access, while SSH can still work inside the local network.

If the Imager provisioning did not succeed, use these recovery steps only:

```bash
sudo apt update
sudo apt install -y rpi-connect-lite
rpi-connect on
rpi-connect signin
```

Open the verification URL printed by `rpi-connect signin`, sign in with the
same Raspberry Pi ID, and assign a unique device name.

## 5. Install Docker Engine

Install the Docker package maintained by Raspberry Pi OS/Debian together with
the tools needed for this project:

```bash
sudo apt update
sudo apt install -y docker.io git make
sudo systemctl enable --now docker
```

Allow the current user to use Docker without `sudo`:

```bash
sudo usermod -aG docker "$USER"
```

Log out completely and reconnect so the new group membership takes effect:

```bash
exit
ssh observer@3d-printer-monitor.local
```

Verify Docker:

```bash
docker info
docker run --rm hello-world
```

Membership in the `docker` group grants root-equivalent control of the host.
Only add trusted administrator accounts.

The project does not require Docker Compose or Buildx. If `docker build` prints
only a legacy-builder deprecation warning, the build can still complete. If a
future Docker version requires Buildx and Raspberry Pi OS provides the package,
install it separately:

```bash
sudo apt install -y docker-buildx
docker buildx version
```

The distribution package can be older than Docker CE, but it provides all
features used by this project: image builds, bind mounts, restart policies, and
container lifecycle commands. Docker also documents its separately maintained
[Docker CE installation](https://docs.docker.com/engine/install/raspberry-pi-os/)
for users who specifically need current upstream releases.

## 6. Download 3d-printer-monitor

```bash
mkdir -p ~/src
cd ~/src
git clone https://github.com/sebastianrau/3d-printer-monitor.git
cd 3d-printer-monitor
```

## 7. Configure the application

```bash
cp config.example.yaml config.yaml
nano config.yaml
```

At minimum, configure:

- `printers[].name`
- `printers[].bambu.host`
- `printers[].bambu.serial`
- `printers[].bambu.access_code`
- `messaging.telegram.bot_token`

Protect the credentials:

```bash
chmod 600 config.yaml
```

The Makefile starts the container with the current user's numeric UID and GID,
so the process can read this owner-only file. The file is mounted read-only and
is excluded from Git and the Docker build context.

If the Telegram chat ID is not known yet, leave `chat_id` empty temporarily,
build the image, and run the discovery command:

```bash
make docker-build
docker run --rm --user "$(id -u):$(id -g)" \
  --volume "$(pwd)/config.yaml:/etc/3d-printer-monitor/config.yaml:ro" \
  3d-printer-monitor:local \
  --config /etc/3d-printer-monitor/config.yaml \
  --find-telegram-chat-id --telegram-wait 60
```

Send `/id` to the bot during the wait period, enter the reported ID in
`config.yaml`, and keep it quoted as a string.

## 8. Build and start the container

```bash
make docker-up
make docker-status
make docker-logs
```

`docker-up` builds a native ARM64 image and creates the `3d-printer-monitor`
container. The configuration is mounted read-only. The
`--restart unless-stopped` policy starts the container after Docker and the Pi
restart, unless it was explicitly stopped.

Press Ctrl+C to leave `docker-logs`; this does not stop the container.

Verify restart behavior:

```bash
sudo reboot
```

After reconnecting:

```bash
make docker-status
make docker-logs
```

## 9. Operations and updates

Useful commands:

```bash
make docker-status
make docker-logs
make docker-stop
make docker-restart
```

Update the application:

```bash
cd ~/src/3d-printer-monitor
git pull --ff-only
make docker-build
make docker-recreate
```

`docker-recreate` removes and replaces only the named application container.
It does not delete `config.yaml`, images, or unrelated containers.

Update the Pi periodically:

```bash
sudo apt update
sudo apt full-upgrade -y
sudo reboot
```

## 10. Troubleshooting

### Docker is not reachable

```bash
systemctl status docker
sudo systemctl restart docker
docker info
groups
```

If `docker info` works only with `sudo`, log out and back in after adding the
user to the `docker` group.

### The container exits immediately

```bash
make docker-status
docker logs 3d-printer-monitor
```

Most startup failures are invalid YAML, missing credentials, an unsupported
printer model, or a configuration file that is not readable by the configured
container UID.

### The printer is unreachable

```bash
ping -c 3 PRINTER_IP
make docker-recreate
make docker-logs
```

Confirm that the Pi and printer can communicate across the LAN and that client
isolation is disabled on the Wi-Fi network. Recheck the printer serial and LAN
access code.

### Raspberry Pi Connect is offline

```bash
rpi-connect status
rpi-connect on
journalctl --user --unit rpi-connect --since today
```

Confirm internet access and that user lingering is enabled:

```bash
loginctl show-user "$USER" --property=Linger
```
