// Browser Flow Integration Test
// TODO.md続編要件に従った実装

const testBrowserFlow = async () => {
  try {
    console.log('🚀 Testing Browser Flow Registration Completion...');
    
    // Step 1: Registration初期化
    const initResponse = await fetch('/api/auth/register', {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json'
      },
      credentials: 'include'
    });
    
    if (!initResponse.ok) {
      throw new Error(`Registration init failed: ${initResponse.status}`);
    }
    
    const initData = await initResponse.json();
    console.log('✅ Registration initialized:', initData.data.id);
    console.log('✅ ui.action:', initData.data.ui.action);
    
    // Step 2: Browser Flow完了 (新しいAPI経由)
    const completionResponse = await fetch(`/api/auth/register/${initData.data.id}`, {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json'
      },
      credentials: 'include',
      body: JSON.stringify({
        email: 'test@example.com',
        password: 'test123456',
        name: 'Test User'
      })
    });
    
    console.log('🎯 Completion response status:', completionResponse.status);
    console.log('🎯 Set-Cookie headers:', completionResponse.headers.getSetCookie?.() || 'Not available');
    
    if (completionResponse.ok) {
      const completionData = await completionResponse.json();
      console.log('✅ Registration completed successfully!');
      console.log('📊 Response data:', completionData);
    } else {
      const errorText = await completionResponse.text();
      console.log('❌ Registration completion failed:', errorText);
    }
    
  } catch (error) {
    console.error('❌ Browser Flow test failed:', error);
  }
};

// Manual execution flag
if (typeof window !== 'undefined') {
  window.testBrowserFlow = testBrowserFlow;
  console.log('Browser Flow test function loaded. Execute: testBrowserFlow()');
}
