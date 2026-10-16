const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const model = require('../assets/journey-model.js');
const journey = () => JSON.parse(fs.readFileSync(require('node:path').join(__dirname,'../../../examples/journey.json'),'utf8'));

test('visual edits preserve all unrelated CLI fields and source objects', () => {
  const c = journey(), original = model.copy(c);
  c.scenarios[0].steps[0].future_field = {preserve:true};
  const changed = model.mutate(c,{type:'edit-step',index:1,patch:{pause:'500ms'}}).config;
  assert.equal(changed.scenarios[0].steps[1].pause,'500ms');
  assert.deepEqual(changed.scenarios[0].data,original.scenarios[0].data);
  assert.deepEqual(changed.scenarios[0].steps[0].future_field,{preserve:true});
  assert.equal(c.scenarios[0].steps[1].pause,undefined);
  changed.scenarios[0].data[0].username='changed';
  assert.equal(c.scenarios[0].data[0].username,'alice');
});
test('request reordering reveals forward references and undo can restore valid order', () => {
  const c=journey();
  assert.deepEqual(model.dependencies(c,0).steps.map(s=>s.missing),[[],[],[]]);
  const moved=model.mutate(c,{type:'move-step',index:1,to:0}).config;
  assert.deepEqual(model.dependencies(moved,0).steps[0].missing,['token']);
  assert.deepEqual(model.dependencies(c,0).edges.map(e=>[e.from,e.to,e.names]),[[0,1,['token']],[0,2,['token']]]);
});
test('a step can assert its extraction but cannot use it in its own request', () => {
  const c={headers:{Authorization:'{{token}}'},scenarios:[{steps:[{url:'/login',headers:{authorization:'public'},extract:{token:{json_pointer:'/token'}},assertions:[{header:'X-Token',equals:'{{token}}'}]}]}]};
  assert.deepEqual(model.dependencies(c,0).steps[0].missing,[]);
  c.scenarios[0].steps[0].body='{{token}}';
  assert.deepEqual(model.dependencies(c,0).steps[0].missing,['token']);
});
test('dependency sources follow variable precedence and latest extraction', () => {
  const c={variables:{id:'global'},scenarios:[{variables:{id:'scenario'},data:[{id:'row'}],steps:[{url:'/one/{{id|path}}',extract:{id:{json_pointer:'/id'}}},{url:'/two/{{id|path}}',extract:{id:{header:'X-ID'}}},{url:'/three/{{id|url}}'}]}]};
  const d=model.dependencies(c,0);
  assert.equal(d.steps[0].before.find(v=>v.name==='id').source,'Dataset row');
  assert.deepEqual(d.edges.map(e=>[e.from,e.to]),[[0,1],[1,2]]);
  assert.deepEqual(model.templates('{{ id | path }} + {{ token }} + {{bad|invalid}}'),['id','token']);
});
test('duplication creates valid unique names without dropping extraction and assertion forms', () => {
  const c=journey();
  const d=model.mutate(c,{type:'duplicate-step',index:0}).config;
  assert.equal(d.scenarios[0].steps[1].name,'Login copy');
  assert.deepEqual(d.scenarios[0].steps[1].assertions,c.scenarios[0].steps[0].assertions);
  assert.deepEqual(d.scenarios[0].steps[1].extract,c.scenarios[0].steps[0].extract);
  assert.equal(model.mutate(d,{type:'duplicate-step',index:0}).config.scenarios[0].steps[1].name,'Login copy 2');
  assert.ok(new TextEncoder().encode(model.unique('界'.repeat(60),[])).length<=128);
});
test('new journeys, conversion, and removal preserve execution limits', () => {
  const c=model.mutate({url:'https://example.test/work',method:'POST',body:'payload',insecure:true},{type:'create'}).config;
  assert.equal(c.scenarios[0].steps[0].url,'https://example.test/work');
  assert.equal(c.scenarios[0].steps[0].body,'payload');
  assert.equal(c.method,'GET');
  assert.equal(c.body,undefined);
  assert.equal(c.insecure,true);
  assert.throws(()=>model.mutate(c,{type:'remove-step',index:0}),/at least one/);
  assert.throws(()=>model.mutate(c,{type:'remove-journey'}),/at least one/);
  const d=model.mutate(c,{type:'add-journey'}).config;
  assert.equal(d.scenarios.length,2);
  assert.equal(model.mutate(d,{type:'remove-journey',scenario:1}).config.scenarios.length,1);
  const large={scenarios:[{steps:Array.from({length:50},(_,i)=>({name:`Step ${i}`,url:'/'}))}]};
  assert.throws(()=>model.mutate(large,{type:'add-step'}),/50 request or control/);
  assert.throws(()=>model.mutate({scenarios:Array.from({length:10},(_,i)=>({name:`Journey ${i}`,weight:1000,steps:[{url:'/'}]}))},{type:'add-journey'}),/10000/);
  assert.throws(()=>model.mutate(c,{type:'edit-journey',patch:{weight:1001}}),/1000/);
});
test('pauses occupy separate positions and malformed JSON shapes are rejected', () => {
  const p=model.layout([{pause:'500ms'},{}]);
  assert.ok(p.nodes[0].pause.y>p.nodes[0].y+p.nodes[0].height);
  assert.ok(p.nodes[1].y>p.nodes[0].pause.y+p.nodes[0].pause.height);
  assert.throws(()=>model.shape({scenarios:[{steps:[{headers:[]}]}]}),/headers/);
  assert.throws(()=>model.shape({scenarios:{}}),/array/);
  for(const patch of [{extract:{bad:null}},{assertions:[{equals:42}]},{headers:{Authorization:42}}])assert.throws(()=>model.shape({scenarios:[{steps:[patch]}]}));
  assert.doesNotThrow(()=>model.shape({variables:null,headers:null,scenarios:[{weight:null,variables:null,data:null,thresholds:null,steps:[{headers:null,extract:null,assertions:null,success_statuses:null}]}]}));
});
test('all edit paths enforce size, step, and weight bounds without changing the source', () => {
  const huge={url:'http://example.test',body:'x'.repeat(1048500)};
  assert.throws(()=>model.mutate(huge,{type:'create'}),/1 MiB/);
  const c={scenarios:Array.from({length:2},(_,i)=>({name:`Journey ${i}`,steps:Array.from({length:50},(_,j)=>({name:`Request ${j}`,url:'/'}))}))};
  assert.throws(()=>model.mutate(c,{type:'duplicate-journey'}),/100 request or control/);
  assert.equal(c.scenarios.length,2);
  assert.throws(()=>model.mutate(journey(),{type:'move-step',index:0,to:-1}),/valid position/);
});

test('nested edits preserve paths, subtrees, and immutable source configuration',()=>{
  const c=journey();let changed=model.mutate(c,{type:'add-branch',index:0}).config;
  changed=model.mutate(changed,{type:'add-loop',index:1,container:'steps[1].if.then'}).config;
  const all=model.flatten(changed.scenarios[0].steps),loop=all.find(n=>n.step.loop);
  assert.equal(loop.path,'steps[1].if.then[1]');
  assert.throws(()=>model.mutate(changed,{type:'move-container',index:loop.index,container:loop.path+'.loop.steps'}),/inside itself/);
  const duplicate=model.mutate(changed,{type:'duplicate-step',index:1}).config;
  assert.equal(duplicate.scenarios[0].steps[2].if.then[1].loop.count,2);
  const names=model.flatten(duplicate.scenarios[0].steps).map(n=>n.step.name);assert.equal(new Set(names).size,names.length);
  const moved=model.mutate(changed,{type:'move-container',index:loop.index,container:'steps'}).config;
  assert.equal(moved.scenarios[0].steps.at(-1).loop.count,2);
  assert.equal(changed.scenarios[0].steps[1].if.then.length,2);assert.equal(c.scenarios[0].steps.length,3);
});
test('dependencies require definitions on both branch paths and scope loop counters',()=>{
  const produce=name=>({name,url:'/',extract:{token:{header:'X-Token'}}});
  const c={scenarios:[{steps:[{if:{condition:{variable:'journey_id',value:'1'},then:[produce('A')],else:[produce('B')]}},{url:'/{{token}}'},{loop:{count:2,index_variable:'i',steps:[{url:'/{{i}}'}]}},{url:'/{{i}}'}]}]};
  const all=model.flatten(c.scenarios[0].steps),d=model.dependencies(c,0);
  assert.deepEqual(d.steps[3].missing,[]);assert.deepEqual(d.edges.filter(e=>e.to===3).map(e=>e.from),[1,2]);
  assert.deepEqual(d.steps[5].missing,[]);assert.deepEqual(d.steps[6].missing,['i']);
  c.scenarios[0].steps[0].if.else=[];assert.deepEqual(model.dependencies(c,0).steps[2].missing,['token']);
  assert.equal(all.length,7);
});
test('nested layout has distinct nodes, labeled branch edges, and loop return paths',()=>{
  const c=JSON.parse(fs.readFileSync(require('node:path').join(__dirname,'../../../examples/flow.json'),'utf8'));
  const layout=model.layout(c.scenarios[0].steps);assert.equal(layout.nodes.length,8);
  assert.ok(layout.edges.some(e=>e.label==='True'));assert.ok(layout.edges.some(e=>e.label==='False'));assert.ok(layout.edges.some(e=>e.label==='Repeat'));
  for(const a of layout.nodes)for(const b of layout.nodes)if(a.index!==b.index)assert.ok(a.x+288<=b.x||b.x+288<=a.x||a.y+a.height<=b.y||b.y+b.height<=a.y,'Node cards must not overlap');
  assert.ok(layout.nodes.every(n=>n.y+n.height<layout.endY));
  const d=model.dependencies(c,0);assert.ok(d.steps.every(s=>s.missing.length===0));
});

test('CSV columns supply canvas dependencies and survive edits',()=>{
  const c={mode:'users',scenarios:[{name:'CSV',csv:{name:'people.csv',content:'Full Name,unused\nalice,x',columns:{'Full Name':'username',unused:''},sharing:'per_user',on_eof:'stop_user'},steps:[{url:'/{{username|path}}'}]}]};
  assert.deepEqual(model.dependencies(c,0).steps[0].missing,[]);
  assert.equal(model.dependencies(c,0).steps[0].before.find(v=>v.name==='username').source,'CSV dataset row');
  const changed=model.mutate(c,{type:'edit-step',index:0,patch:{pause:'1ms'}}).config;
  assert.deepEqual(changed.scenarios[0].csv,c.scenarios[0].csv);
  const removed=model.mutate(changed,{type:'edit-journey',patch:{csv:null}}).config;
  assert.deepEqual(model.dependencies(removed,0).steps[0].missing,['username']);
  assert.throws(()=>model.shape({scenarios:[{steps:[],csv:{content:123}}]}),/CSV/);
});

test('native CSV default header mappings also supply dependency hints',()=>{
  for(const csv of [
    {content:'\ufeff\r\n"username",item\r\nalice,tea'},
    {content:'username;item\nalice;tea',delimiter:';'},
    {content:'"Full\nName",username\nx,alice',columns:{'Full\nName':''}},
  ])assert.deepEqual(model.dependencies({scenarios:[{csv,steps:[{url:'/{{username|path}}'}]}]},0).steps[0].missing,[]);
});
