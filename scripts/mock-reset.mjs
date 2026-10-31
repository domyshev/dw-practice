const base=`http://127.0.0.1:${process.env.MOCK_API_PORT||8081}`;
const origin=process.env.MOCK_APP_ORIGIN||`http://127.0.0.1:${process.env.MOCK_WEB_PORT||5174}`;
const health=await fetch(`${base}/api/health`).then(r=>r.json());
if(health.mock!==true)throw Error('Refusing to reset a server that is not explicitly in mock mode');
const response=await fetch(`${base}/api/mock/reset`,{method:'POST',headers:{'Content-Type':'application/json',Origin:origin},body:'{}'});
const result=await response.json();if(!response.ok)throw Error(result.error||'Mock reset failed');
console.log('Mock fixtures restored. Reload the browser and sign in with demo@dwpractice.test / 123456.');
