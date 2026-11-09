# Offline Hub clock and RTC

The operator can check clock evidence locally with `stagecore clock`. The
command reads the Linux `rtc0` timestamp and kernel boot evidence, and checks
that the RTC and wall clock are plausible and within five seconds. It does
not need NTP or a network request. An optional fresh RFC3339 `--reference`
from an independently trusted local clock verifies absolute time as well.

Use `stagecore clock --require-rtc-boot` immediately after a disconnected
cold boot. Record its JSON output. Both the kernel RTC boot flag and time
agreement must pass. Boot evidence identifies use of the RTC, while a fresh
reference or a known-good clock recorded before power-off checks absolute
accuracy. Qualify the actual Pi RTC, battery and driver before relying on it.

An authorized local administrator can explicitly use `--write-rtc` with
`--reference` to sync the RTC from a verified system clock and read it back.
The write requires root, an RTC exposed as `/dev/rtc0`, a UTC RTC and
util-linux `hwclock`. If the wall clock disagrees with the fresh reference,
the command refuses to write. Correct the system clock through the existing
maintenance procedure before trying again. StageCore never steps the wall
clock or changes the system time-sync configuration through this command.

- [Linux RTC sysfs interface](https://kernel.googlesource.com/pub/scm/linux/kernel/git/torvalds/linux.git/+/refs/tags/v4.0/Documentation/rtc.txt)
- [util-linux hwclock manual](https://man7.org/linux/man-pages/man8/hwclock.8.html)
