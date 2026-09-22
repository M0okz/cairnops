"""Synthetic API fixtures: no credentials or production requests."""

import json
from copy import deepcopy
from datetime import datetime, timezone, timedelta

now = datetime.now(timezone.utc).isoformat()
targets = [
    {
        "id": str(i),
        "name": name,
        "description": "",
        "created_at": now,
        "external_source_count": 2,
        "aliases": [],
        "sources": [],
        "category": cat,
        "suggested_category": cat,
        "category_manual": False,
    }
    for i, (name, cat) in enumerate(
        [
            ("Home Assistant", "service"),
            ("Serveur de stockage", "infrastructure"),
            ("Sauvegarde quotidienne", "scheduled_task"),
            ("Vaultwarden", "software"),
            ("Contrôle à identifier", "unclassified"),
        ]
    )
]


def incident(i, target, nature, title, severity):
    evidence = {
        "id": i,
        "impact_id": i,
        "target_id": target,
        "origin": "zabbix",
        "name": title,
        "active": True,
        "severity": severity,
        "opened_at": now,
        "last_seen_at": now,
        "upstream_acknowledged": False,
        "acknowledgement_sync_status": "not_applicable",
    }
    return {
        "id": i,
        "nature_key": nature,
        "nature_label": title,
        "nature_scope": "connector",
        "nature_namespace": "test",
        "nature_fingerprint": nature,
        "status": "active",
        "severity": severity,
        "opened_at": now,
        "last_impact_at": now,
        "propagation_status": "closed",
        "active_impact_count": 1,
        "impact_count": 1,
        "affected_target_count": 1,
        "impacts": [
            {
                "id": i,
                "target_id": target,
                "target_name": targets[int(target)]["name"],
                "status": "active",
                "effective_severity": severity,
                "source_severity": severity,
                "opened_at": now,
                "maintenance_active": False,
                "evidence": [evidence],
            }
        ],
        "activity": [],
    }


incidents = [
    incident("a", "0", "version-query", "Version installée non relevée", "critical"),
    incident("b", "0", "backup-old", "Sauvegarde trop ancienne", "warning"),
    incident(
        "c", "0", "software-update-available", "Mise à jour disponible", "warning"
    ),
]
measure = {
    "window": "24h",
    "availability": 1,
    "coverage": 0.95,
    "average_latency_milliseconds": 14,
    "maximum_latency_milliseconds": 22,
    "conclusive_observations": 100,
    "unknown_observations": 0,
    "expected_observations": 105,
}
metrics = [
    {
        "target_id": t["id"],
        "measures": [measure],
        "trend": [1] * 24,
        "latency_trend": [12, 14, 18, 13, 14],
        "latest_observed_at": now,
        "sources": [
            {
                "source_id": "k" + t["id"],
                "name": "Accès HTTP",
                "kind": "uptime_kuma",
                "origin": "integration",
                "measures_availability": t["category"] == "service",
                "latest_outcome": "healthy",
                "latest_observed_at": now,
                "measures": [measure],
            }
        ],
    }
    for t in targets
]

user = {
    "id": "qa",
    "username": "gregory",
    "display_name": "Grégory",
    "role": "administrator",
    "authorization_regime": "local",
    "created_at": now,
    "active_sessions": 2,
    "last_seen_at": now,
    "deactivated_at": None,
    "external_suspended_at": None,
}
connectors = [
    {
        "id": kind,
        "kind": kind,
        "name": name,
        "endpoint": "https://" + kind + ".example.test",
        "status": "connected",
        "remote_version": "7.0.20" if kind == "zabbix" else "",
        "compatibility": "supported",
        "encrypted_transport": True,
        "binding_count": 12,
        "quarantine_count": 0,
        "last_checked_at": now,
        "created_at": now,
        "updated_at": now,
    }
    for kind, name in [
        ("zabbix", "Zabbix"),
        ("argus", "Argus"),
        ("uptime_kuma", "Uptime Kuma"),
    ]
]
services = [
    {
        "id": "software",
        "target_id": "0",
        "name": "Home Assistant",
        "installed_version": "2026.8.1",
        "target_version": "2026.9.1",
        "observed_at": now,
        "known": True,
        "source": {
            "kind": "github",
            "url": "https://github.com/home-assistant/core",
            "software": "Home Assistant",
        },
        "source_origin": "argus",
        "suggested_source": None,
        "confirmed_at": now,
        "revision": 1,
        "state": "awaiting_ai",
        "last_error": "",
        "checked_at": now,
        "collection": None,
        "collection_revision": None,
        "analyses": [],
        "history": [
            {
                "installed_version": "2026.8.1",
                "target_version": "2026.9.1",
                "observed_at": now,
            }
        ],
    }
]


def payload(path):
    default = {
        "initialized": True,
        "name": "Homeblack",
        "enabled": False,
        "operations": [],
        "suggestions": [],
        "services": [],
        "incoming": [],
        "connectors": [],
        "incidents": [],
        "days": [],
        "maintenances": [],
        "channels": [],
        "entries": [],
        "unread": 0,
        "targets": [],
        "user": user,
        "users": [user],
        "devices": [],
        "active_sessions": 2,
        "sources": [],
        "measures": [],
        "indicators": [],
        "activity": [],
        "observations": [],
        "target_id": "0",
        "draft": None,
        "active": None,
        "bindings": [],
        "profiles": [],
    }
    if path.endswith("/indicators") and "/incidents/" in path:
        return {
            "incident_id": "a",
            "target_ids": ["0"],
            "opened_at": now,
            "snapshots": [],
            "indicators": [],
            "series": {},
            "disclaimer": "Mesures de contexte",
        }
    if path == "/api/v1/connectors/zabbix/preview":
        return {
            "kind": "zabbix",
            "name": "Zabbix",
            "endpoint": "https://zabbix.example.test",
            "version": "7.0.20",
            "compatibility": "supported",
            "compatibility_label": "Compatible",
            "encrypted_transport": True,
            "hosts": [
                {
                    "external_id": "host-1",
                    "name": "Serveur de stockage",
                    "technical_name": "storage-01",
                    "interfaces": [],
                    "candidate_targets": [],
                }
            ],
            "available_targets": [{"id": t["id"], "name": t["name"]} for t in targets],
            "access": {"mode": "provided", "will_provision": False},
            "receipt": "qa",
            "expires_at": (
                datetime.now(timezone.utc) + timedelta(minutes=15)
            ).isoformat(),
        }
    if "/indicator-configuration" in path:
        return {
            "connector_id": "zabbix",
            "connector_kind": "zabbix",
            "connector_name": "Zabbix",
            "endpoint": "https://zabbix.example.test",
            "generated_at": now,
            "capabilities": [],
            "profiles": [],
            "activity": [],
            "bindings": [
                {
                    "external_id": "host-1",
                    "external_name": "Serveur de stockage",
                    "enabled": True,
                    "imported": True,
                    "target_id": "1",
                    "indicators": [],
                    "candidates": [
                        {
                            "external_id": "cpu-1",
                            "semantic_key": "cpu.utilization",
                            "dimension": "",
                            "label": "Utilisation du processeur",
                            "unit": "percent",
                            "available": True,
                            "recommended": True,
                        }
                    ],
                }
            ],
        }
    if path.startswith("/api/v1/device-pairings"):
        pairing = {
            "id": "qa",
            "status": "awaiting_scan",
            "expires_at": (
                datetime.now(timezone.utc) + timedelta(minutes=5)
            ).isoformat(),
            "created_at": now,
        }
        return (
            {
                "pairing": pairing,
                "instance_url": "https://cairnops.example.test",
                "token": "synthetic-qa-only",
                "qr_payload": "https://cairnops.example.test/pair/qa",
            }
            if path == "/api/v1/device-pairings"
            else pairing
        )
    if path == "/api/v1/software-update-settings":
        return {
            "enabled": False,
            "endpoint": "https://api.example.test/v1",
            "model": "qa",
            "key_configured": False,
        }
    if path == "/api/v1/targets":
        return {"targets": targets}
    if path == "/api/v1/targets/0":
        return targets[0]
    if path == "/api/v1/incidents":
        return {"incidents": incidents}
    if path.startswith("/api/v1/incidents/") and path.split("/")[-1] in ["a", "b", "c"]:
        return next(i for i in incidents if i["id"] == path.split("/")[-1])
    if path == "/api/v1/metrics/targets":
        return {"targets": metrics}
    if path == "/api/v1/targets/0/metrics":
        return metrics[0]
    if path == "/api/v1/connectors":
        return {"connectors": connectors}
    if path == "/api/v1/software-updates":
        return {"services": services}
    if path == "/api/v1/software-updates/software":
        return services[0]
    if path == "/api/v1/system/health":
        return {
            "status": "operational",
            "checked_at": now,
            "components": [
                {
                    "name": n,
                    "status": "operational",
                    "detail": "Opérationnel",
                    "instances": 1,
                    "checked_at": now,
                }
                for n in ["server", "worker", "postgresql", "push"]
            ],
            "hours": [],
            "database": {
                "latency_milliseconds": 2,
                "maximum_latency_milliseconds": 5,
                "samples": [1, 2, 3],
                "measured_since": now,
            },
        }
    if path == "/api/v1/maintenances":
        return {
            "maintenances": [
                {
                    "id": "maint",
                    "name": "Maintenance du stockage",
                    "reason": "Mise à jour prévue",
                    "state": "upcoming",
                    "starts_at": now,
                    "ends_at": (
                        datetime.now(timezone.utc) + timedelta(hours=2)
                    ).isoformat(),
                    "targets": [{"id": "0", "name": "Home Assistant"}],
                    "created_at": now,
                }
            ]
        }
    if path in ["/api/v1/indicators/targets", "/api/v1/indicators/catalog"]:
        return {
            "targets": [
                {
                    "target_id": "0",
                    "generated_at": now,
                    "indicators": [
                        {
                            "id": "latency",
                            "target_id": "0",
                            "semantic_key": "response.time",
                            "label": "Temps de réponse",
                            "unit": "milliseconds",
                            "enabled": True,
                            "pinned": True,
                            "last_observed_at": now,
                            "metadata": {},
                        }
                    ],
                    "series": {
                        "latency": [
                            {
                                "at": (
                                    datetime.now(timezone.utc)
                                    - timedelta(minutes=(60 - i) * 5)
                                ).isoformat(),
                                "value": 20 + (i % 5) * 7,
                            }
                            for i in range(61)
                        ]
                    },
                }
            ]
        }
    return default


def install(page, theme="light", scenario="normal", locale="fr"):
    from urllib.parse import urlparse

    available_version = ["0.1.112"]

    def handle(route):
        path = urlparse(route.request.url).path
        data = deepcopy(payload(path))
        if scenario == "long" and path == "/api/v1/targets":
            for target in data["targets"]:
                target["name"] = (
                    "Serveur-de-sauvegarde-avec-un-nom-particulièrement-long-sur-plusieurs-sites"
                )

        if path == "/api/v1/version":
            data = {"version": available_version[0]}
        if scenario == "empty" and path in [
            "/api/v1/targets",
            "/api/v1/incidents",
            "/api/v1/connectors",
            "/api/v1/maintenances",
            "/api/v1/software-updates",
            "/api/v1/metrics/targets",
            "/api/v1/indicators/targets",
        ]:
            data = {key: [] for key in data}
        if scenario == "error" and path in [
            "/api/v1/software-updates",
            "/api/v1/users",
            "/api/v1/devices",
        ]:
            route.fulfill(
                status=503,
                content_type="application/json",
                body=json.dumps(
                    {
                        "error": "Service momentanément indisponible. Réessayez dans quelques instants."
                    }
                ),
            )
            return
        route.fulfill(
            status=200, content_type="application/json", body=json.dumps(data)
        )

    page.route("**/api/**", handle)
    page.add_init_script(
        f"localStorage.setItem('cairnops-locale','{locale}');localStorage.setItem('cairnops-theme','{theme}');"
    )

    def publish_update():
        # Switch only after boot: concurrent startup responses must agree.
        available_version[0] = "0.1.113"

    return publish_update
