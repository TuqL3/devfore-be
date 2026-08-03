# Image for the Networking course labs.
#
# The sandbox runs with `--network none`, so there is exactly one interface:
# loopback. That is less of a limit than it sounds — a socket bound to
# 127.0.0.1 is a real socket, a real port and a real TCP handshake, so
# client/server, listening ports and HTTP requests are all learnable in here.
# What is genuinely out of reach: another host, ICMP (ping needs CAP_NET_RAW,
# and every capability is dropped), DNS, and any route beyond `lo`.
#
# Built with `labs/` as the context — see labs/linux/Dockerfile.
FROM alpine:3.21

# iproute2 for ip/ss, busybox-extras for the httpd and telnet applets, curl to
# speak HTTP at it. Everything else is the Linux image's toolkit.
RUN apk add --no-cache bash coreutils findutils grep less nano \
        iproute2 curl busybox-extras \
    && adduser -D -u 1000 student

COPY bashrc /etc/devforge.bashrc

USER student
WORKDIR /home/student

CMD ["sleep", "infinity"]
