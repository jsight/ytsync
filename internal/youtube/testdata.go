package youtube

// SampleYtdlpOutput is a sample yt-dlp JSON response for a small channel
const SampleYtdlpOutput = `{
  "id": "UCuAXFkgsw1L7xaCfnd5JJOw",
  "title": "Test Channel",
  "uploader": "Test Uploader",
  "uploader_id": "test_uploader",
  "channel_id": "UCuAXFkgsw1L7xaCfnd5JJOw",
  "channel_url": "https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw",
  "description": "Test channel description",
  "entries": [
    {
      "id": "dQw4w9WgXcQ",
      "title": "Video 1",
      "description": "First video",
      "duration": 212,
      "view_count": 1000000,
      "uploader": "Test Uploader",
      "uploader_id": "test_uploader",
      "channel_id": "UCuAXFkgsw1L7xaCfnd5JJOw",
      "upload_date": "20200101",
      "timestamp": 1577836800,
      "thumbnail": "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg"
    },
    {
      "id": "xQw4w9WgXcZ",
      "title": "Video 2",
      "description": "Second video",
      "duration": 180,
      "view_count": 500000,
      "uploader": "Test Uploader",
      "uploader_id": "test_uploader",
      "channel_id": "UCuAXFkgsw1L7xaCfnd5JJOw",
      "upload_date": "20200102",
      "timestamp": 1577923200,
      "thumbnail": "https://i.ytimg.com/vi/xQw4w9WgXcZ/maxresdefault.jpg"
    }
  ]
}`

// SampleAtomFeed is a sample YouTube Atom/RSS feed
const SampleAtomFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/">
  <title>YouTube Channel Videos</title>
  <link rel="alternate" href="https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw"/>
  <link rel="self" href="https://www.youtube.com/feeds/videos.xml?channel_id=UCuAXFkgsw1L7xaCfnd5JJOw"/>
  <author>
    <name>Test Uploader</name>
    <uri>https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw</uri>
  </author>
  <published>2020-01-02T12:00:00-05:00</published>
  <entry>
    <id>yt:video:dQw4w9WgXcQ</id>
    <yt:videoId>dQw4w9WgXcQ</yt:videoId>
    <yt:channelId>UCuAXFkgsw1L7xaCfnd5JJOw</yt:channelId>
    <title>Video 1</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=dQw4w9WgXcQ"/>
    <author>
      <name>Test Uploader</name>
      <uri>https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw</uri>
    </author>
    <published>2020-01-01T00:00:00Z</published>
    <updated>2020-01-02T00:00:00Z</updated>
    <media:group>
      <media:description>First video</media:description>
      <media:thumbnail url="https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" width="480" height="360"/>
      <media:community>
        <media:statistics views="1000000"/>
      </media:community>
    </media:group>
  </entry>
  <entry>
    <id>yt:video:xQw4w9WgXcZ</id>
    <yt:videoId>xQw4w9WgXcZ</yt:videoId>
    <yt:channelId>UCuAXFkgsw1L7xaCfnd5JJOw</yt:channelId>
    <title>Video 2</title>
    <link rel="alternate" href="https://www.youtube.com/watch?v=xQw4w9WgXcZ"/>
    <author>
      <name>Test Uploader</name>
      <uri>https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw</uri>
    </author>
    <published>2020-01-02T00:00:00Z</published>
    <updated>2020-01-02T00:00:00Z</updated>
    <media:group>
      <media:description>Second video</media:description>
      <media:thumbnail url="https://i.ytimg.com/vi/xQw4w9WgXcZ/hqdefault.jpg" width="480" height="360"/>
      <media:community>
        <media:statistics views="500000"/>
      </media:community>
    </media:group>
  </entry>
</feed>`

// SampleEmptyAtomFeed is a sample YouTube Atom feed with no entries
const SampleEmptyAtomFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/">
  <title>YouTube Channel Videos</title>
  <link rel="alternate" href="https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw"/>
  <author>
    <name>Test Uploader</name>
    <uri>https://www.youtube.com/channel/UCuAXFkgsw1L7xaCfnd5JJOw</uri>
  </author>
  <published>2020-01-02T12:00:00-05:00</published>
</feed>`
