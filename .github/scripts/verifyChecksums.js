const fs = require('fs');
const path = require('path');
const https = require('https');

const feedDir = '../../feed';

function httpsGet(url) {
    return new Promise((resolve, reject) => {
        https.get(url, (res) => {
            let data = '';
            res.on('data', (chunk) => data += chunk);
            res.on('end', () => resolve(data));
        }).on('error', (err) => reject(err));
    });
}

async function verifyChecksumLink(link, checksumLink) {
    try {
        const data = await httpsGet(checksumLink);
        const archiveName = link.split('/').pop();
        if (data.includes(archiveName)) {
            console.log(`✅  Verified: ${link}`);
        } else {
            console.error(`❌  Checksum does not contain archive name for ${link}`);
        }
    } catch (error) {
        console.error(`Error fetching ${checksumLink}: ${error.message}`);
    }
}

async function verifyChecksums(product) {
    console.log(`::group::Product Code: ${product.Code}`);
    for (const release of product.Releases) {
        for (const downloadInfo of Object.values(release.Downloads)) {
            await verifyChecksumLink(downloadInfo.Link, downloadInfo.ChecksumLink);
        }
    }
    console.log('::endgroup::');
}

async function verifyAllReleases() {
    const files = fs.readdirSync(feedDir).filter(f => f.endsWith('.releases.json'));

    for (const file of files) {
        const filePath = path.join(feedDir, file);
        const content = fs.readFileSync(filePath, 'utf8');
        const product = JSON.parse(content);
        await verifyChecksums(product);
    }
}

verifyAllReleases().catch(err => {
    console.error(`Error: ${err.message}`);
    process.exit(1);
});
