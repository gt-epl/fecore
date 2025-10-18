# Makefile for zpipe

PREFIX=$(shell pwd)

TARGET=compression-n

CFLAGS += -I $(PREFIX)/.. -Wall

LDFLAGS += -L $(PREFIX)

ZPIPE_OBJ = zpipe.o

LIBS += -lz

all: $(TARGET)

.c.o:
	$(CC) -c $(CFLAGS) $< -o $@

$(TARGET): $(ZPIPE_OBJ)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $(TARGET) $(ZPIPE_OBJ) $(LIBS)

clean:
	rm -f *~ *.o $(TARGET)

install:
	mkdir -p $(PREFIX)/bin
	cp -f $(TARGET) $(PREFIX)/bin/$(TARGET)
