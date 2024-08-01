// Copyright 2022 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import {UserPreferences} from '@parca/components';

const SettingsPage = () => {
  return (
    <section>
      <div className="bg-white dark:bg-gray-700 max-w-[800px] p-10 w-[800px] mx-auto mt-[60px] rounded">
        <h1 className="text-3xl dark:text-gray-100 font-bold">Visualisation Setttings</h1>
        <div>
          <UserPreferences />
        </div>
      </div>
    </section>
  );
};

export default SettingsPage;
