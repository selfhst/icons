import express from 'express'
import fetch from 'node-fetch'
import path from 'path'

const app = express()
const PORT = 4050
const CDN_ROOT = 'https://cdn.jsdelivr.net/gh/selfhst/icons'
const CDN_PATH = 'svg'

async function fileExists(url) {
  try {
    const resp = await fetch(url, { method: 'HEAD' });
    return resp.ok;
  } catch {
    return false;
  }
}

async function fetchAndPipe(url, res) {
  const response = await fetch(url);
  if (!response.ok) return res.status(404).send('File not found');
  res.type(path.extname(url).slice(1));
  response.body.pipe(res);
}

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
    return fetchAndPipe(`${CDN_ROOT}/${CDN_PATH}/${filename}`, res);
  }

  let mainUrl;
  if (ext === 'png') {
	  mainUrl = `${CDN_ROOT}/png/${filename}`;
  } else if (ext === 'webp') {
	  mainUrl = `${CDN_ROOT}/webp/${filename}`;
  } else if (ext === 'svg') { 
      mainUrl = `${CDN_ROOT}/svg/${filename}`;
  } else {
	  mainUrl = null; 
  }
  
  const hasColor = !!req.query['color'] && req.query['color'].trim() !== '';

  if (ext === 'svg') {
    if (hasColor) {
      const baseName = filename.replace(/\.(png|webp|svg)$/, '');
      const suffixUrl = `${CDN_ROOT}/${CDN_PATH}/${baseName}-light.svg`;
      if (await fileExists(suffixUrl)) {
        let svgContent = await fetch(suffixUrl).then(r => r.text());
        const color = req.query['color'].startsWith('#') ? req.query['color'] : `#${req.query['color']}`;
        svgContent = svgContent
          .replace(/style="[^"]*fill:\s*#fff[^"]*"/gi, (match) => {
            console.log('Replacing style fill:', match);
            return match.replace(/fill:\s*#fff/gi, `fill:${color}`);
          })
          .replace(/fill="#fff"/gi, `fill="${color}"`);
        return res.type('image/svg+xml').send(svgContent);
      } else {
        return fetchAndPipe(mainUrl, res);
      }
    } else {
      return fetchAndPipe(mainUrl, res);
    }
  } else {
    // PNG/WebP: serve directly
    return fetchAndPipe(mainUrl, res);
  }
});

app.get('/', (req, res) => {
  res.send('Self-hosted icon server');
});
app.listen(PORT, () => {
  console.log(`Listening on port ${PORT}`);
});