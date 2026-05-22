#!/bin/sh
# Integration test fixture: Redis seed data
redis-cli SET "session:health-check" "active"
redis-cli SET "cache:homepage" "rendered-html"
redis-cli SET "user:1:name" "alice"
redis-cli SET "user:2:name" "bob"
redis-cli SET "counter:visits" "42"
