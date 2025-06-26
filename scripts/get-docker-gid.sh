#!/bin/bash
# Get Docker group ID dynamically
getent group docker | cut -d: -f3 || echo "999"