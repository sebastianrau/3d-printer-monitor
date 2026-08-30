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

Optionally reserve the Pi's address in the router's DHCP settings. A stable
address makes administration easier, although 3d-printer-monitor itself does not
listen on a network port.

## 3. Install Docker Engine

Install the Docker package maintained by Raspberry Pi OS/Debian together with
`curl`, which is used once to download the configuration template:

```bash
sudo apt update
sudo apt install -y docker.io curl
sudo systemctl enable --now docker
```

Allow the current user to use Docker without `sudo`:

```bash
sudo usermod -aG docker "$USER"
```

Raspberry Pi Connect does not reliably preserve a shell across the required
session refresh. Reboot instead, then reconnect:

```bash
sudo reboot
```

After the reboot, reconnect through Raspberry Pi Connect or SSH.

Verify Docker:

```bash
docker info
docker run --rm hello-world
```

Membership in the `docker` group grants root-equivalent control of the host.
Only add trusted administrator accounts.

The distribution package can be older than Docker CE, but it provides all
features used by this project: image pulls, bind mounts, restart policies, and
container lifecycle commands. Docker also documents its separately maintained
[Docker CE installation](https://docs.docker.com/engine/install/raspberry-pi-os/)
for users who specifically need current upstream releases.

## 4. Install and configure 3d-printer-monitor

The quickest installation uses the deployment script:

```bash
curl -fsSL https://raw.githubusercontent.com/sebastianrau/3d-printer-monitor/main/install.sh | sh
```

On its first run, the script creates
`~/3d-printer-monitor/config.yaml` with owner-only permissions and exits. This
user-owned location avoids requiring root privileges for routine configuration
changes while still protecting the secrets with mode `600`. `/etc` would be
appropriate for a system-managed package, but adds unnecessary ownership and
permission handling for this user-managed Docker installation.

Edit the new file:

```bash
nano ~/3d-printer-monitor/config.yaml
```

At minimum, configure:

- `printers[].name`
- `printers[].bambu.host`
- `printers[].bambu.serial`
- `printers[].bambu.access_code`
- `messaging.telegram.bot_token`

Then run the same installer command again. It pulls the matching ARM image,
creates the container, mounts the configuration read-only, and configures
automatic restart. Existing configurations are never overwritten.

An existing configuration can be used without copying it:

```bash
CONFIG_PATH=/path/to/config.yaml sh install.sh
```

The application itself is delivered as a container image. A Git checkout and
Go toolchain are not required on the Pi.

If the Telegram chat ID is not known yet, leave `chat_id` empty temporarily,
run the installer once to pull the image, stop the monitor to avoid two bot
pollers, and run the discovery command:

```bash
docker stop 3d-printer-monitor
docker run --rm --user "$(id -u):$(id -g)" \
  --volume "$HOME/3d-printer-monitor/config.yaml:/etc/3d-printer-monitor/config.yaml:ro" \
  ghcr.io/sebastianrau/3d-printer-monitor:latest \
  --config /etc/3d-printer-monitor/config.yaml \
  --find-telegram-chat-id --telegram-wait 60
```

Send `/id` to the bot during the wait period, enter the reported ID in
`~/3d-printer-monitor/config.yaml`, keep it quoted as a string, and run the
installer again to start normal monitoring.

## 5. Operations and application updates

The installer is also the normal update command. It pulls the configured image
and safely replaces the existing named container without changing
`config.yaml`:

```bash
curl -fsSL https://raw.githubusercontent.com/sebastianrau/3d-printer-monitor/main/install.sh | sh
```

Useful commands:

```bash
docker ps --filter name=3d-printer-monitor
docker logs --follow 3d-printer-monitor
docker stop 3d-printer-monitor
docker restart 3d-printer-monitor
```

## 6. Update Raspberry Pi OS

Run operating-system upgrades through SSH, not a Raspberry Pi Connect shell.
Package upgrades can restart the Raspberry Pi Connect service, which terminates
its remote shell and can interrupt the command that is performing the upgrade.

Connect through SSH and update the Pi:

```bash
ssh observer@3d-printer-monitor.local
sudo apt update
sudo apt full-upgrade -y
sudo reboot
```

## 7. Raspberry Pi Connect (optional)

The Pi should appear at
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
