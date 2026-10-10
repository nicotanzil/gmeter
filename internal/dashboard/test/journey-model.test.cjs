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
  assert.throws(()=>model.mutate(large,{type:'add-step'}),/50 request/);
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
  assert.throws(()=>model.mutate(c,{type:'duplicate-journey'}),/100 request/);
  assert.equal(c.scenarios.length,2);
  assert.throws(()=>model.mutate(journey(),{type:'move-step',index:0,to:-1}),/valid position/);
});
