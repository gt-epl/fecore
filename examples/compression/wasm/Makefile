# Makefile for zpipe wasm version

PREFIX=/src/wasm

TARGET=zpipe.wasm

CFLAGS += -I /src/zlib -Wall

# libz.a in this directory is the WASM version of zlib library
LDFLAGS += -L /src/wasm

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
