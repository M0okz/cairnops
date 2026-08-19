CREATE UNIQUE INDEX cairnops_incident_signals_uptime_kuma_active_idx
    ON cairnops_incident_signals (connector_binding_id)
    WHERE origin = 'uptime_kuma' AND active;
