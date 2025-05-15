import fs from 'fs'
import express from 'express'
import path from 'path'

const app = express()
const PORT = 4050

const APP_DIR = process.cwd()

async function readFile(filePath, res) {
  const readStream = fs.createReadStream(filePath);

  readStream.on("open", () => {
    res.type(path.extname(filePath).slice(1));
    readStream.pipe(res);
  });
  readStream.on("error", () => {
    res.set("Content-Type", "text/plain");
    res.status(404).end("Not found");
  });
}

app.get('/', (_req, res) => {
  res.send('Self-hosted icon server');
});

app.get('/*', async (req, res) => {
  const urlPath = req.path;
  const extMatch = urlPath.match(/\.(\w+)$/);
  if (!extMatch)
    return res.status(404).send('File extension missing');

  const ext = extMatch[1].toLowerCase();
  if (!['png', 'webp', 'svg'].includes(ext))
    return res.status(404).send('Format not supported');

  const filename = urlPath.slice(1);
  const lowerFilename = filename.toLowerCase();

  const isSuffix = lowerFilename.endsWith('-light.svg') || lowerFilename.endsWith('-dark.svg');

  if (isSuffix) {
    return readFile(`${APP_DIR}/svg/${filename}`, res);
  }

  let filePath;
  if (ext === 'png') {
    filePath = `${APP_DIR}/png/${filename}`;
  } else if (ext === 'webp') {
    filePath = `${APP_DIR}/webp/${filename}`;
  } else if (ext === 'svg') {
    filePath = `${APP_DIR}/svg/${filename}`;
  } else {
    filePath = null;
  }

  const hasColor = !!req.query['color'] && req.query['color'].trim() !== '';

  if (ext === 'svg') {
    if (hasColor) {
      const baseName = filename.replace(/\.(png|webp|svg)$/, '');
      const baseFile = `${APP_DIR}/svg/${baseName}-light.svg`;

      if (fs.existsSync(baseFile)) {
        let svgContent = fs.readFileSync(baseFile, { encoding: 'utf-8'});
        const color = req.query['color'].startsWith('#') ? req.query['color'] : `#${req.query['color']}`;
        svgContent = svgContent
          .replace(/style="[^"]*fill:\s*#fff[^"]*"/gi, (match) => {
            console.log('Replacing style fill:', match);
            return match.replace(/fill:\s*#fff/gi, `fill:${color}`);
          })
          .replace(/fill="#fff"/gi, `fill="${color}"`);
        return res.type('image/svg+xml').send(svgContent);
      } else {
        return res.status(404).end("Not found");
      }
    } else {
      return readFile(filePath, res);
    }
  } else {
    // PNG/WebP: serve directly
    return readFile(filePath, res);
  }
});

app.listen(PORT, () => {
  console.log(`Listening on port ${PORT}`);
});