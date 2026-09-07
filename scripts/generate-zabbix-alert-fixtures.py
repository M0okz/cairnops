"""Regenerate presentation fixtures from pinned official templates.

Requires PyYAML in a disposable Python environment. This script is development
tooling, never a runtime classifier. Review every new mapping before adding it.
"""
import hashlib
import json
from pathlib import Path
from urllib.request import urlopen

import yaml

REVISIONS = {
    "6.0": "4519972900f52106a17bf1a223f4e4029165970e",
    "7.0": "66dddcedc71f98b20acf630f9b15b0e955fd41e9",
    "7.4": "6c0e5ee881b6556ef74c040d4825e5f140df9489",
}
MEANINGS = {
    "Cert: SSL certificate expires soon": "certificate.expiring",
    "Cert: SSL certificate is invalid": "certificate.invalid",
    "Cert [{#CERT.WEBSITE.ITEMNAME}]: SSL certificate expires soon": "certificate.expiring",
    "Cert [{#CERT.WEBSITE.ITEMNAME}]: SSL certificate is invalid": "certificate.invalid",
    "Certificate: SSL certificate expires soon": "certificate.expiring",
    "Certificate: SSL certificate is invalid": "certificate.invalid",
    "SSL certificate expires soon": "certificate.expiring",
    "SSL certificate is invalid": "certificate.invalid",
    "{#DEVNAME}: Disk read/write request responses are too high": "disk.latency.high",
    "{#FSNAME}: Disk space is critically low": "disk.space.low",
    "{#FSNAME}: Disk space is low": "disk.space.low",
    "{#FSNAME}: Running out of free inodes": "disk.inodes.low",
    "Linux: High CPU utilization": "cpu.usage.high",
    "Linux: High memory utilization": "memory.usage.high",
    "Linux: Lack of available memory": "memory.available.low",
    "Linux: High swap space usage": "swap.space.low",
    "Linux: Load average is too high": "system.load.high",
    "Linux: Number of installed packages has been changed": "packages.count.changed",
    "Linux: {#DEVNAME}: Disk read/write request responses are too high": "disk.latency.high",
    "Linux: FS [{#FSNAME}]: Space is critically low": "disk.space.low",
    "Linux: FS [{#FSNAME}]: Space is low": "disk.space.low",
    "Linux: FS [{#FSNAME}]: Running out of free inodes": "disk.inodes.low",
}


def triggers(value):
    if isinstance(value, dict):
        if "uuid" in value and "expression" in value:
            yield value
        for child in value.values():
            yield from triggers(child)
    elif isinstance(value, list):
        for child in value:
            yield from triggers(child)


root = Path(__file__).resolve().parents[1]
fixtures = []
for version, revision in REVISIONS.items():
    for path in (
        "templates/os/linux/template_os_linux.yaml",
        "templates/os/linux_active/template_os_linux_active.yaml",
        "templates/app/certificate_agent2/template_app_certificate_agent2.yaml",
    ):
        url = f"https://raw.githubusercontent.com/zabbix/zabbix/{revision}/{path}"
        raw = urlopen(url, timeout=30).read()
        for trigger in triggers(yaml.safe_load(raw)):
            kind = MEANINGS.get(trigger["name"])
            if not kind:
                continue
            fixtures.append({
                "version": version, "source": url,
                "source_sha256": hashlib.sha256(raw).hexdigest(),
                "kind": kind,
                "trigger": {
                    "uuid": trigger["uuid"], "description": trigger["name"],
                    "expression": trigger["expression"],
                    "recovery_expression": trigger.get("recovery_expression", ""),
                },
            })
destination = root / "internal/connectors/zabbix/standard_alerts.json"
destination.write_text(json.dumps(fixtures, ensure_ascii=False, indent=2) + "\n")
print(f"Wrote {len(fixtures)} official trigger fixtures to {destination}")
