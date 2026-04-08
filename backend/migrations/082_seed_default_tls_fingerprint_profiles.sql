-- Seed 3 default TLS fingerprint profiles based on real Claude Code client captures.
-- These profiles provide fingerprint diversity across different Node.js versions and platforms.
-- Uses INSERT ... ON CONFLICT to be idempotent (safe to re-run).

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- Profile 1: macOS arm64 / Node.js v24.x (with GREASE)
-- Based on captured macOS arm64 Node.js v24.3.0 fingerprint, GREASE enabled.
-- GREASE inserts random 0x?a?a placeholders into cipher suites and
-- TLS extension-related vectors, producing a distinct JA3/JA4 variant.
-- Base shape: 17 ciphers, 3 curves, 14 extensions + GREASE bookends.
INSERT INTO tls_fingerprint_profiles (name, description, enable_grease,
    cipher_suites, curves, point_formats, signature_algorithms,
    alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions)
VALUES (
    'macOS arm64 Node.js v24',
    'Claude Code on macOS arm64 with Node.js v24.x — GREASE enabled, BoringSSL-derived stack (17 base ciphers, 3 base curves, 14+2 extensions)',
    true,
    '[4865,4866,4867,49195,49199,49196,49200,52393,52392,49161,49171,49162,49172,156,157,47,53]',
    '[29,23,24]',
    '[0]',
    '[1027,2052,1025,1283,2053,1281,2054,1537,513]',
    '["http/1.1"]',
    '[772,771]',
    '[29]',
    '[1]',
    '[0,65037,23,65281,10,11,35,16,5,13,18,51,45,43]'
) ON CONFLICT (name) DO NOTHING;

-- Profile 2: Linux x64 / Node.js v22.17.1
-- Significantly different fingerprint: 57 cipher suites, 10 curves (incl. ffdhe groups),
-- 20 signature algorithms, 11 extensions (no ECH, with encrypt_then_mac).
-- OpenSSL 3.2+ based TLS stack.
-- JA4 cipher hash: a33745022dd6
INSERT INTO tls_fingerprint_profiles (name, description, enable_grease,
    cipher_suites, curves, point_formats, signature_algorithms,
    alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions)
VALUES (
    'Linux x64 Node.js v22',
    'Claude Code on Linux x64 with Node.js v22.x — OpenSSL 3.2 stack with broad cipher/curve support (57 ciphers, 10 curves, 20 sig algs)',
    false,
    '[4866,4867,4865,49199,49195,49200,49196,158,49191,103,49192,107,163,159,52393,52392,52394,49327,49325,49315,49311,49245,49249,49239,49235,162,49326,49324,49314,49310,49244,49248,49238,49234,49188,106,49187,64,49162,49172,57,56,49161,49171,51,50,157,49313,49309,49233,156,49312,49308,49232,61,60,53,47,255]',
    '[29,23,30,25,24,256,257,258,259,260]',
    '[0,1,2]',
    '[1027,1283,1539,2055,2056,2057,2058,2059,2052,2053,2054,1025,1281,1537,771,769,770,1026,1282,1538]',
    '["http/1.1"]',
    '[772,771]',
    '[29]',
    '[1]',
    '[0,11,10,35,16,22,23,13,43,45,51]'
) ON CONFLICT (name) DO NOTHING;

-- Profile 3: Linux x64 / Node.js v20.x (LTS)
-- OpenSSL 3.0.x based stack — between v22 (broad) and v24 (BoringSSL-lean).
-- 23 ciphers (AES-256 prioritized, includes SHA-256/SHA-384 CBC variants),
-- 5 curves (X25519, P-256, X448, P-521, P-384 — no ffdhe groups),
-- 12 extensions (no ECH, with encrypt_then_mac, different order from v22),
-- 11 signature algorithms (includes ed25519).
INSERT INTO tls_fingerprint_profiles (name, description, enable_grease,
    cipher_suites, curves, point_formats, signature_algorithms,
    alpn_protocols, supported_versions, key_share_groups, psk_modes, extensions)
VALUES (
    'Linux x64 Node.js v20',
    'Claude Code on Linux x64 with Node.js v20.x LTS — OpenSSL 3.0 stack, AES-256 prioritized (23 ciphers, 5 curves, 12 extensions)',
    false,
    '[4866,4867,4865,49196,49200,52393,52392,49195,49199,49188,49192,49187,49191,49162,49172,49161,49171,157,156,61,60,53,47]',
    '[29,23,30,25,24]',
    '[0]',
    '[1027,1283,1539,2055,2052,2053,2054,1025,1281,1537,513]',
    '["http/1.1"]',
    '[772,771]',
    '[29]',
    '[1]',
    '[0,23,65281,10,11,35,22,16,13,43,45,51]'
) ON CONFLICT (name) DO NOTHING;
