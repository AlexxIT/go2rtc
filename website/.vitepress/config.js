import {defineConfig} from 'vitepress';

function replace_link(md) {
    md.core.ruler.after('inline', 'replace-link', function (state) {
        for (const block of state.tokens) {
            if (block.type === 'inline' && block.children) {
                for (const token of block.children) {
                    const href = token.attrGet('href');
                    if (href && href.indexOf('README.md') >= 0) {
                        // token.attrJoin('style', 'color:red;');
                        token.attrSet('href', href.replace('README.md', 'index.md'));
                    }
                }
            }
        }
        return true;
    });
}

export default defineConfig({
    title: 'go2rtc',
    description: 'Ultimate camera streaming application',
    head: [
        // first line (green bold) of Telegram card, autodetect from hostname
        ['meta', { property: 'og:site_name', content: 'go2rtc.org' }],
        // second line of Telegram card (black bold), autodetect from site description
        ['meta', { property: 'og:title', content: 'go2rtc - Ultimate camera streaming application' }],
        // third line of Telegram card, autodetect from site description
        ['meta', { property: 'og:description', content: 'Support alsa, doorbird, dvrip, eseecloud, ffmpeg, gopro, hass, hls, homekit, mjpeg, mp4, mpegts, nest, onvif, ring, roborock, rtmp, rtsp, tapo, vigi, tuya, v4l2, webrtc, wyze, xiaomi.' }],
        ['meta', { property: 'og:url', content: 'https://go2rtc.org/' }],
        ['meta', { property: 'og:image', content: 'https://go2rtc.org/images/logo.png' }],
        // important for Telegram - the image will be at the bottom and large
        ['meta', { property: 'twitter:card', content: 'summary_large_image' }],
    ],

    themeConfig: {
        nav: [
            {text: 'Home', link: '/'},
        ],
        sidebar: [
            {
                items: [
                    {text: 'Installation', link: '/#installation'},
                    {text: 'Configuration', link: '/#configuration'},
                    {text: 'Security', link: '/#security'},
                ],
            },
            {
                text: 'Features',
                items: [
                    {text: 'Streaming input', link: '/#streaming-input'},
                    {text: 'Streaming output', link: '/#streaming-output'},
                    {text: 'Streaming ingest', link: '/#streaming-ingest'},
                    {text: 'Two-way audio', link: '/#two-way-audio'},
                    {text: 'Stream to camera', link: '/#stream-to-camera'},
                    {text: 'Publish stream', link: '/#publish-stream'},
                    {text: 'Preload stream', link: '/#preload-stream'},
                    {text: 'Streaming stats', link: '/#streaming-stats'},
                ],
                collapsed: false,
            },
            {
                text: 'Codecs',
                items: [
                    {text: 'Codecs filters', link: '/#codecs-filters'},
                    {text: 'Codecs madness', link: '/#codecs-madness'},
                    {text: 'Built-in transcoding', link: '/#built-in-transcoding'},
                    {text: 'Codecs negotiation', link: '/#codecs-negotiation'},
                ],
                collapsed: true,
            },
            {
                text: 'Other',
                items: [
                    {text: 'Projects using go2rtc', link: '/#projects-using-go2rtc'},
                    {text: 'Camera experience', link: '/#camera-experience'},
                    {text: 'Tips', link: '/#tips'},
                ],
                collapsed: true,
            },
            {
                text: 'Core modules',
                items: [
                    {text: 'app', link: '/internal/app/'},
                    {text: 'api', link: '/internal/api/'},
                    {text: 'streams', link: '/internal/streams/'},
                ],
                collapsed: false,
            },
            {
                text: 'Main modules',
                items: [
                    {text: 'http', link: '/internal/http/'},
                    {text: 'mjpeg', link: '/internal/mjpeg/'},
                    {text: 'mp4', link: '/internal/mp4/'},
                    {text: 'rtsp', link: '/internal/rtsp/'},
                    {text: 'webrtc', link: '/internal/webrtc/'},
                ],
                collapsed: false,
            },
            {
                text: 'Other modules',
                items: [
                    {text: 'hls', link: '/internal/hls/'},
                    {text: 'homekit', link: '/internal/homekit/'},
                    {text: 'onvif', link: '/internal/onvif/'},
                    {text: 'rtmp', link: '/internal/rtmp/'},
                    {text: 'webtorrent', link: '/internal/webtorrent/'},
                    {text: 'wyoming', link: '/internal/wyoming/'},
                ],
                collapsed: false,
            },
            {
                text: 'Script sources',
                items: [
                    {text: 'echo', link: '/internal/echo/'},
                    {text: 'exec', link: '/internal/exec/'},
                    {text: 'expr', link: '/internal/expr/'},
                    {text: 'ffmpeg', link: '/internal/ffmpeg/'},
                ],
                collapsed: false,
            },
            {
                text: 'Other sources',
                items: [
                    {text: 'alsa', link: '/internal/alsa/'},
                    {text: 'bubble', link: '/internal/bubble/'},
                    {text: 'doorbird', link: '/internal/doorbird/'},
                    {text: 'dvrip', link: '/internal/dvrip/'},
                    {text: 'eseecloud', link: '/internal/eseecloud/'},
                    {text: 'flussonic', link: '/internal/flussonic/'},
                    {text: 'gopro', link: '/internal/gopro/'},
                    {text: 'hass', link: '/internal/hass/'},
                    {text: 'isapi', link: '/internal/isapi/'},
                    {text: 'ivideon', link: '/internal/ivideon/'},
                    {text: 'kasa', link: '/internal/kasa/'},
                    {text: 'mpeg', link: '/internal/mpeg/'},
                    {text: 'multitrans', link: '/internal/multitrans/'},
                    {text: 'nest', link: '/internal/nest/'},
                    {text: 'ring', link: '/internal/ring/'},
                    {text: 'roborock', link: '/internal/roborock/'},
                    {text: 'tapo', link: '/internal/tapo/'},
                    {text: 'tuya', link: '/internal/tuya/'},
                    {text: 'v4l2', link: '/internal/v4l2/'},
                    {text: 'wyze', link: '/internal/wyze/'},
                    {text: 'xiaomi', link: '/internal/xiaomi/'},
                    {text: 'yandex', link: '/internal/yandex/'},
                ],
                collapsed: false,
            },
            {
                text: 'Helper modules',
                items: [
                    {text: 'debug', link: '/internal/debug/'},
                    {text: 'ngrok', link: '/internal/ngrok/'},
                    {text: 'pinggy', link: '/internal/pinggy/'},
                    {text: 'srtp', link: '/internal/srtp/'},
                ],
                collapsed: false,
            },

        ],
        socialLinks: [
            {icon: 'github', link: 'https://github.com/AlexxIT/go2rtc'}
        ],
        outline: [2, 3],
        search: {provider: 'local'},
    },

    rewrites(id) {
        // change file names
        return id.replace('README.md', 'index.md');
    },

    markdown: {
        config: (md) => {
            // change markdown links
            md.use(replace_link);
        }
    },

    srcDir: '..',
    srcExclude: ['examples/', 'pkg/'],

    // cleanUrls: true,
    ignoreDeadLinks: true,
});
