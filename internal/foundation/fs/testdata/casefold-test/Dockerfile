# syntax=docker/dockerfile:1
FROM debian:bookworm-slim
RUN <<-EOF
	apt-get update -qq
	apt-get install -y -qq dosfstools
	rm -rf /var/lib/apt/lists/*
EOF
