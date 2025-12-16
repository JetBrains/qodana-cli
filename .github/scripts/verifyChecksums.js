const fs = require('fs');
const https = require('https');

const jsonFilePath = '../../feed/releases.json';

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

async function verifyChecksums(data) {
    for (const product of data) {
        console.log(`::group::Product Code: ${product.Code}`);
        for (const release of product.Releases) {
            for (const downloadInfo of Object.values(release.Downloads)) {
                await verifyChecksumLink(downloadInfo.Link, downloadInfo.ChecksumLink);
            }
        }
        console.log('::endgroup::');
    }
}

fs.readFile(jsonFilePath, 'utf8', async (err, data) => {
    if (err) {
        console.error(`Error reading file from disk: ${err}`);
    } else {
        const releasesData = JSON.parse(data);
        await verifyChecksums(releasesData);
    }
});
