-- Las altas nuevas tienen avatar sin necesitar una importación previa de Sheets.
-- El legado incluye DefaultH0..5 y solo DefaultF0: los niveles femeninos usan
-- ese archivo disponible hasta que el administrador suba uno por nivel.
-- Una instalación existente conserva todos sus avatares personalizados.
insert into avatars (code, level, url)
select defaults.code, levels.level,
  '/media/legacy/avatars/' || defaults.code ||
  case when defaults.code = 'DefaultF' then '0' else levels.level::text end || '.png'
from (values ('DefaultH'), ('DefaultF')) as defaults(code)
cross join generate_series(0, 5) as levels(level)
on conflict (code, level) do nothing;
