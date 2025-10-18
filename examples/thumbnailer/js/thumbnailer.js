var http = require('http')
const sharp = require('sharp')
path = require('path')
var fs = require('fs');

function process(input_path, width, height) {
    var startTime = performance.now();
    sharp(input_path).resize(width, height).png().ensureAlpha().toFile("output.png");
    var endTime = performance.now();
    var elapsed = endTime - startTime;
    return elapsed.toPrecision(4);
};

http.createServer(function (req, res) {
    sharp.concurrency(1);
    var elapsed = process("./input.png", 256, 193)
    var ret = Math.round(elapsed);
    res.setHeader('invocation-elapsed', ret);
    res.writeHead(200, {'Content-Type': 'text/plain'});
    res.end(ret + '\n');
}).listen(8080, '0.0.0.0');
