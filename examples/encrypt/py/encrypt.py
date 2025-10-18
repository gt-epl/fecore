#!/usr/bin/env python3
import datetime
import random
import string
import pyaes
# Web server based on: https://gist.github.com/nitaku/10d0662536f37a087e1b
from http.server import BaseHTTPRequestHandler, HTTPServer
import socketserver
import json
import cgi
import sys
import subprocess

def fn():
    start = datetime.datetime.now()

    num_of_iterations = 10000
    message = b'hello world'
    # 128-bit key (16 bytes)
    KEY = b'fecoreencryption'

    aes = pyaes.AESModeOfOperationCTR(KEY)
    for loops in range(num_of_iterations):
        ciphertext = aes.encrypt(message)

    end = datetime.datetime.now()
    diff = end - start
    invocation_elapsed = int(diff.total_seconds() * 1000)
    return invocation_elapsed

class Server(BaseHTTPRequestHandler):
    def _set_headers(self):
        self.send_response(200)
        self.send_header('invocation-elapsed', self.invocation_elapsed)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()

    def do_HEAD(self):
        self._set_headers()

    def do_GET(self):
        self.invocation_elapsed = fn()
        self._set_headers()
        respbody = bytes(str(self.invocation_elapsed), 'ascii')
        self.wfile.write(respbody)

def run(server_class=HTTPServer, handler_class=Server, port=8080):
    server_address = ('', port)
    httpd = server_class(server_address, handler_class)

    httpd.serve_forever()

if __name__ == "__main__":
    run()
