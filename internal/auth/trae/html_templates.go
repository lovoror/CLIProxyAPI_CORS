package trae

import "fmt"

// AuthHelperHtml is the bridge page for Trae authentication.
// It guides the user on how to extract their app-token and provides a form to submit it back to the local proxy.
const AuthHelperHtml = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Trae 身份认证助手</title>
    <style>
        body { font-family: -apple-system, sans-serif; background: #f4f7f6; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; }
        .card { background: white; padding: 2rem; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,0.08); max-width: 500px; width: 100%; }
        h1 { font-size: 1.5rem; color: #333; margin-top: 0; }
        p { color: #666; line-height: 1.6; }
        code { background: #eee; padding: 0.2rem 0.4rem; border-radius: 4px; font-family: monospace; }
        .step { margin-bottom: 1.5rem; }
        .step-label { font-weight: bold; color: #007bff; display: block; margin-bottom: 0.5rem; }
        textarea, input { width: 100%; padding: 0.8rem; margin-top: 0.5rem; border: 1px solid #ddd; border-radius: 6px; box-sizing: border-box; }
        button { background: #007bff; color: white; border: none; padding: 0.8rem 1.5rem; border-radius: 6px; cursor: pointer; font-size: 1rem; width: 100%; margin-top: 1rem; }
        button:hover { background: #0056b3; }
        .footer { margin-top: 1.5rem; font-size: 0.8rem; color: #999; text-align: center; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Trae 身份认证</h1>
        <div class="step">
            <span class="step-label">第一步：登录 Trae</span>
            <p>在浏览器中打开 <a href="https://www.trae.ai" target="_blank">trae.ai</a> 并登录您的账户。</p>
        </div>
        <div class="step">
            <span class="step-label">第二步：提取 App Token</span>
            <p>按下 <code>F12</code> 打开开发者工具，切换到 <code>Network</code> (网络) 标签，随便找一个接口请求，在请求头 (Request Headers) 中找到 <code>app-token</code> 或从 Cookie 中找到类似字段。</p>
        </div>
        <div class="step">
            <span class="step-label">第三步：填入 Token</span>
            <form id="authForm">
                <input type="text" id="email" placeholder="邮箱 (可选，用于区分账号)" />
                <textarea id="token" rows="4" placeholder="请在此粘贴您的 app-token" required></textarea>
                <button type="submit">完成认证</button>
            </form>
        </div>
        <div class="footer">认证成功后，本页面将自动尝试关闭并通知代理服务器。</div>
    </div>

    <script>
        const urlParams = new URLSearchParams(window.location.search);
        const state = urlParams.get('state');

        document.getElementById('authForm').onsubmit = function(e) {
            e.preventDefault();
            const token = document.getElementById('token').value.trim();
            const email = document.getElementById('email').value.trim();
            const port = %d;
            
            if (!token) {
                alert("请输入 Token");
                return;
            }

            // 构建回调 URL
            const callbackUrl = 'http://localhost:' + port + '/trae-callback?token=' + encodeURIComponent(token) + '&email=' + encodeURIComponent(email) + '&state=' + encodeURIComponent(state);
            
            // 跳转到本地回调地址（由代理服务器开启的 forwarder 监听）
            window.location.href = callbackUrl;
        };
    </script>
</body>
</html>`

// GetAuthHelperHtml returns the HTML with the specific local port injected.
func GetAuthHelperHtml(port int) string {
	return fmt.Sprintf(AuthHelperHtml, port)
}
