-- Datos iniciales: los mismos valores que el código viejo traía fijos. Desde aquí todo se
-- cambia en el panel de administración.

insert into levels (level, min_exp) values (0, 0), (1, 10), (2, 25), (3, 50), (4, 75), (5, 100);

-- Los tres primeros son los que mostraba el selector del login. Com y POO existían en el
-- servidor pero no en el selector; quedan desactivados con las imágenes que usaban antes.
insert into courses (name, sheet_name, sheet_aliases, side_image_url, logo_url, hidden_badge_keywords, active, sort_order) values
  ('Técnicas Avanzadas de Representación', 'TAR',
   array['Técnicas Avanzadas de Representación', 'tecnicas', 'técnicas', 'tar'],
   '/media/legacy/img/side-image.jpg', '/media/legacy/logo.png', array[]::text[], true, 1),
  ('Analisis de Datos', 'Analisis de Datos',
   array['Análisis de Datos', 'Analisis de datos', 'Análisis de datos', 'analisis', 'datos'],
   '/media/legacy/img/poo-side.jpg', '/media/legacy/img/ad-01.png', array['internet', 'calculadora'], true, 2),
  ('Interfaces Hombre-Máquina', 'IHM',
   array['Interfaces Hombre-Máquina', 'interfaces', 'ihm'],
   '/media/legacy/img/side-image.jpg', '/media/legacy/logo.png', array[]::text[], true, 3),
  ('Diseno de la Comunicacion', 'Com',
   array['Diseño de la Comunicación', 'diseno de la comunicacion', 'com'],
   '/media/legacy/img/comuni.png', '/media/legacy/img/Com.png', array[]::text[], false, 4),
  ('POO', 'POO',
   array['poo'],
   '/media/legacy/img/poo-side.jpg', '/media/legacy/img/poo-logo.png', array[]::text[], false, 5);
