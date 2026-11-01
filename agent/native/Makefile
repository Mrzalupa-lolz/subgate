CC ?= cc
CFLAGS ?= -O2 -Wall -Wextra -Werror -D_DEFAULT_SOURCE
NDPI_FLAGS := $(shell pkg-config --atleast-version=4.14 libndpi && echo -DSUBGATE_NDPI_414)

subgate-ndpi: ndpi-collector.c
	$(CC) $(CFLAGS) $(NDPI_FLAGS) $< -o $@ $$(pkg-config --cflags --libs libndpi libpcap)

clean:
	rm -f subgate-ndpi
